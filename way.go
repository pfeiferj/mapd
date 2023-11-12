package main

import (
	"errors"
	"math"
)

type OnWayResult struct {
	OnWay    bool
	Distance DistanceResult
}

func OnWay(way Way, lat float64, lon float64) (OnWayResult, error) {
	res := OnWayResult{}
	if lat < way.MaxLat()+PADDING && lat > way.MinLat()-PADDING && lon < way.MaxLon()+PADDING && lon > way.MinLon()-PADDING {
		d, err := DistanceToWay(lat, lon, way)
		res.Distance = d
		if err != nil {
			res.OnWay = false
			return res, err
		}
		lanes := way.Lanes()
		if lanes == 0 {
			lanes = 2
		}
		road_width_estimate := float64(lanes) * LANE_WIDTH
		max_dist := 5 + road_width_estimate

		if d.Distance < max_dist {
			res.OnWay = true
			return res, nil
		}
	}
	res.OnWay = false
	return res, nil
}

type DistanceResult struct {
	LineStart Coordinates
	LineEnd   Coordinates
	Distance  float64
}

func DistanceToWay(lat float64, lon float64, way Way) (DistanceResult, error) {
	res := DistanceResult{}
	var minNodeStart Coordinates
	var minNodeEnd Coordinates
	minDistance := math.MaxFloat64
	nodes, err := way.Nodes()
	if err != nil {
		return res, err
	}
	if nodes.Len() < 2 {
		return res, nil
	}

	latRad := lat * TO_RADIANS
	lonRad := lon * TO_RADIANS
	for i := 0; i < nodes.Len()-1; i++ {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		lineLat, lineLon := PointOnLine(nodeStart.Latitude(), nodeStart.Longitude(), nodeEnd.Latitude(), nodeEnd.Longitude(), lat, lon)
		distance := DistanceToPoint(latRad, lonRad, lineLat*TO_RADIANS, lineLon*TO_RADIANS)
		if distance < minDistance {
			minDistance = distance
			minNodeStart = nodeStart
			minNodeEnd = nodeEnd
		}
	}
	res.Distance = minDistance
	res.LineStart = minNodeStart
	res.LineEnd = minNodeEnd
	return res, nil
}

type CurrentWay struct {
	Way      Way
	Distance DistanceResult
}

func GetCurrentWay(state *State, offline Offline, lat float64, lon float64) (CurrentWay, error) {
	if state.CurrentWay.Way.HasNodes() {
		onWay, err := OnWay(state.CurrentWay.Way, lat, lon)
		loge(err)
		if onWay.OnWay {
			return CurrentWay{
				Way:      state.CurrentWay.Way,
				Distance: onWay.Distance,
			}, nil
		}
	}

	// check the expected next way
	if state.NextWay.HasNodes() {
		onWay, err := OnWay(state.NextWay, lat, lon)
		loge(err)
		if onWay.OnWay {
			return CurrentWay{
				Way:      state.NextWay,
				Distance: onWay.Distance,
			}, nil
		}
	}

	// finally check all other ways
	ways, err := offline.Ways()
	if err != nil {
		return CurrentWay{}, err
	}
	for i := 0; i < ways.Len(); i++ {
		way := ways.At(i)
		onWay, err := OnWay(way, lat, lon)
		loge(err)
		if onWay.OnWay {
			return CurrentWay{
				Way:      way,
				Distance: onWay.Distance,
			}, nil
		}
	}

	return CurrentWay{}, errors.New("COULD NOT FIND WAY")
}

func IsForward(currentWay CurrentWay, bearing float64) bool {
	startLat := currentWay.Distance.LineStart.Latitude()
	startLon := currentWay.Distance.LineStart.Longitude()
	endLat := currentWay.Distance.LineEnd.Latitude()
	endLon := currentWay.Distance.LineEnd.Longitude()

	wayBearing := Bearing(startLat, startLon, endLat, endLon)
	bearingDelta := math.Abs(bearing*TO_RADIANS - wayBearing)
	return math.Cos(bearingDelta) >= 0
}

func MatchingWays(currentWay CurrentWay, offline Offline, matchNode Coordinates) ([]Way, error) {
	matchingWays := []Way{}
	ways, err := offline.Ways()
	if err != nil {
		return matchingWays, err
	}

	for i := 0; i < ways.Len(); i++ {
		w := ways.At(i)
		if !w.HasNodes() {
			continue
		}

		if w.MinLat() == currentWay.Way.MinLat() && w.MaxLat() == currentWay.Way.MaxLat() && w.MinLon() == currentWay.Way.MinLon() && w.MaxLon() == currentWay.Way.MaxLon() {
			continue
		}

		wNodes, err := w.Nodes()
		if err != nil {
			return matchingWays, err
		}
		if wNodes.Len() < 2 {
			continue
		}

		fNode := wNodes.At(0)
		lNode := wNodes.At(wNodes.Len() - 1)
		if (fNode.Latitude() == matchNode.Latitude() && fNode.Longitude() == matchNode.Longitude()) || (lNode.Latitude() == matchNode.Latitude() && lNode.Longitude() == matchNode.Longitude()) {
			matchingWays = append(matchingWays, w)
		}
	}

	return matchingWays, nil
}

func NextWay(currentWay CurrentWay, offline Offline, lat float64, lon float64, bearing float64) (Way, Coordinates, error) {
	nodes, err := currentWay.Way.Nodes()
	if err != nil {
		return Way{}, Coordinates{}, err
	}
	if !currentWay.Way.HasNodes() || nodes.Len() == 0 {
		return Way{}, Coordinates{}, nil
	}

	var matchNode Coordinates
	if IsForward(currentWay, bearing) {
		matchNode = nodes.At(nodes.Len() - 1)
	} else {
		matchNode = nodes.At(0)
	}

	matchingWays, err := MatchingWays(currentWay, offline, matchNode)
	if err != nil {
		return Way{}, matchNode, err
	}

	if len(matchingWays) == 0 {
		return Way{}, matchNode, nil
	}

	// first return if one of the next connecting ways has the same name
	name, _ := currentWay.Way.Name()
	if len(name) > 0 {
		for _, mWay := range matchingWays {
			mName, err := mWay.Name()
			if err != nil {
				return Way{}, matchNode, err
			}
			if mName == name {
				return mWay, matchNode, nil
			}
		}
	}

	// second return if one of the next connecting ways has the same refs
	ref, _ := currentWay.Way.Ref()
	if len(ref) > 0 {
		for _, mWay := range matchingWays {
			mRef, err := mWay.Ref()
			if err != nil {
				return Way{}, matchNode, err
			}
			if mRef == ref {
				return mWay, matchNode, nil
			}
		}
	}

	// third return the next connecting way with the least change in bearing
	minDiffWay := matchingWays[0]
	minDiff := float64(0)
	for _, mWay := range matchingWays {
		nodes, err := mWay.Nodes()
		if err != nil {
			continue
		}

		var bearingNode Coordinates
		if matchNode == nodes.At(0) {
			bearingNode = nodes.At(1)
		} else {
			bearingNode = nodes.At(nodes.Len() - 2)
		}

		mBearing := Bearing(matchNode.Latitude(), matchNode.Longitude(), bearingNode.Latitude(), bearingNode.Longitude())
		bearingDiff := math.Abs(math.Abs(bearing*TO_RADIANS) - math.Abs(mBearing))

		if bearingDiff < minDiff {
			minDiff = bearingDiff
			minDiffWay = mWay
		}
	}

	return minDiffWay, matchNode, nil
}
