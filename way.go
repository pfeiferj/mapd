package main

import (
	"errors"
	"math"
)

type OnWayResult struct {
	OnWay     bool
	Distance  DistanceResult
	IsForward bool
}

func OnWay(way Way, pos Position) (OnWayResult, error) {
	res := OnWayResult{}
	if pos.Latitude < way.MaxLat()+PADDING && pos.Latitude > way.MinLat()-PADDING && pos.Longitude < way.MaxLon()+PADDING && pos.Longitude > way.MinLon()-PADDING {
		d, err := DistanceToWay(pos, way)
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
			res.IsForward = IsForward(d.LineStart, d.LineEnd, pos.Bearing)
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

func DistanceToWay(pos Position, way Way) (DistanceResult, error) {
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

	latRad := pos.Latitude * TO_RADIANS
	lonRad := pos.Longitude * TO_RADIANS
	for i := 0; i < nodes.Len()-1; i++ {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		lineLat, lineLon := PointOnLine(nodeStart.Latitude(), nodeStart.Longitude(), nodeEnd.Latitude(), nodeEnd.Longitude(), pos.Latitude, pos.Longitude)
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
	Way           Way
	Distance      DistanceResult
	OnWay         OnWayResult
	StartPosition Coordinates
	EndPosition   Coordinates
}

func GetWayStartEnd(way Way, isForward bool) (Coordinates, Coordinates) {
	if !way.HasNodes() {
		return Coordinates{}, Coordinates{}
	}

	nodes, err := way.Nodes()
	if err != nil {
		loge(err)
		return Coordinates{}, Coordinates{}
	}

	if nodes.Len() == 0 {
		return Coordinates{}, Coordinates{}
	}

	if nodes.Len() == 1 {
		return nodes.At(0), nodes.At(0)
	}

	if isForward {
		return nodes.At(0), nodes.At(nodes.Len() - 1)
	}
	return nodes.At(nodes.Len() - 1), nodes.At(0)
}

func GetCurrentWay(currentWay Way, nextWay Way, offline Offline, pos Position) (CurrentWay, error) {
	if currentWay.HasNodes() {
		onWay, err := OnWay(currentWay, pos)
		loge(err)
		if onWay.OnWay {
			start, end := GetWayStartEnd(currentWay, onWay.IsForward)
			return CurrentWay{
				Way:           currentWay,
				Distance:      onWay.Distance,
				OnWay:         onWay,
				StartPosition: start,
				EndPosition:   end,
			}, nil
		}
	}

	// check the expected next way
	if nextWay.HasNodes() {
		onWay, err := OnWay(nextWay, pos)
		loge(err)
		if onWay.OnWay {
			start, end := GetWayStartEnd(nextWay, onWay.IsForward)
			return CurrentWay{
				Way:           nextWay,
				Distance:      onWay.Distance,
				OnWay:         onWay,
				StartPosition: start,
				EndPosition:   end,
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
		onWay, err := OnWay(way, pos)
		loge(err)
		if onWay.OnWay {
			start, end := GetWayStartEnd(way, onWay.IsForward)
			return CurrentWay{
				Way:           way,
				Distance:      onWay.Distance,
				OnWay:         onWay,
				StartPosition: start,
				EndPosition:   end,
			}, nil
		}
	}

	return CurrentWay{}, errors.New("COULD NOT FIND WAY")
}

func IsForward(lineStart Coordinates, lineEnd Coordinates, bearing float64) bool {
	startLat := lineStart.Latitude()
	startLon := lineStart.Longitude()
	endLat := lineEnd.Latitude()
	endLon := lineEnd.Longitude()

	wayBearing := Bearing(startLat, startLon, endLat, endLon)
	bearingDelta := math.Abs(bearing*TO_RADIANS - wayBearing)
	return math.Cos(bearingDelta) >= 0
}

func MatchingWays(currentWay Way, offline Offline, matchNode Coordinates) ([]Way, error) {
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

		if w.MinLat() == currentWay.MinLat() && w.MaxLat() == currentWay.MaxLat() && w.MinLon() == currentWay.MinLon() && w.MaxLon() == currentWay.MaxLon() {
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

type NextWayResult struct {
	Way           Way
	IsForward     bool
	StartPosition Coordinates
	EndPosition   Coordinates
}

func NextIsForward(nextWay Way, matchNode Coordinates) bool {
	if !nextWay.HasNodes() {
		return true
	}
	nodes, err := nextWay.Nodes()
	if err != nil || nodes.Len() < 2 {
		loge(err)
		return true
	}

	lastNode := nodes.At(nodes.Len() - 1)
	if lastNode.Latitude() == matchNode.Latitude() && lastNode.Longitude() == matchNode.Longitude() {
		return false
	}

	return true
}

func NextWay(currentWay CurrentWay, offline Offline, lat float64, lon float64, bearing float64) (NextWayResult, error) {
	nodes, err := currentWay.Way.Nodes()
	if err != nil {
		return NextWayResult{}, err
	}
	if !currentWay.Way.HasNodes() || nodes.Len() == 0 {
		return NextWayResult{}, nil
	}

	var matchNode Coordinates
	if currentWay.OnWay.IsForward {
		matchNode = nodes.At(nodes.Len() - 1)
	} else {
		matchNode = nodes.At(0)
	}

	matchingWays, err := MatchingWays(currentWay.Way, offline, matchNode)
	if err != nil {
		return NextWayResult{StartPosition: matchNode}, err
	}

	if len(matchingWays) == 0 {
		return NextWayResult{StartPosition: matchNode}, nil
	}

	// first return if one of the next connecting ways has the same name
	name, _ := currentWay.Way.Name()
	if len(name) > 0 {
		for _, mWay := range matchingWays {
			mName, err := mWay.Name()
			if err != nil {
				return NextWayResult{StartPosition: matchNode}, err
			}
			if mName == name {
				isForward := NextIsForward(mWay, matchNode)
				start, end := GetWayStartEnd(mWay, isForward)
				return NextWayResult{
					Way:           mWay,
					StartPosition: start,
					EndPosition:   end,
					IsForward:     isForward,
				}, nil
			}
		}
	}

	// second return if one of the next connecting ways has the same refs
	ref, _ := currentWay.Way.Ref()
	if len(ref) > 0 {
		for _, mWay := range matchingWays {
			mRef, err := mWay.Ref()
			if err != nil {
				return NextWayResult{StartPosition: matchNode}, err
			}
			if mRef == ref {
				isForward := NextIsForward(mWay, matchNode)
				start, end := GetWayStartEnd(mWay, isForward)
				return NextWayResult{
					Way:           mWay,
					StartPosition: start,
					EndPosition:   end,
					IsForward:     isForward,
				}, nil
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

	isForward := NextIsForward(minDiffWay, matchNode)
	start, end := GetWayStartEnd(minDiffWay, isForward)
	return NextWayResult{
		Way:           minDiffWay,
		StartPosition: start,
		EndPosition:   end,
		IsForward:     NextIsForward(minDiffWay, matchNode),
	}, nil
}
