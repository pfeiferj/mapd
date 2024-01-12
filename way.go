package main

import (
	"math"

	"github.com/pkg/errors"
)

var MIN_WAY_DIST = 500 // meters. how many meters to look ahead before stopping gathering next ways.

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
			return res, errors.Wrap(err, "could not get distance to way")
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
		return res, errors.Wrap(err, "could not read way nodes")
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
		logde(errors.Wrap(err, "could not read way nodes"))
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

func GetCurrentWay(currentWay Way, nextWays []NextWayResult, offline Offline, pos Position) (CurrentWay, error) {
	if currentWay.HasNodes() {
		onWay, err := OnWay(currentWay, pos)
		logde(errors.Wrap(err, "could not check if on current way"))
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

	// check the expected next ways
	for _, nextWay := range nextWays {
		onWay, err := OnWay(nextWay.Way, pos)
		logde(errors.Wrap(err, "could not check if on next way"))
		if onWay.OnWay {
			start, end := GetWayStartEnd(nextWay.Way, onWay.IsForward)
			return CurrentWay{
				Way:           nextWay.Way,
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
		return CurrentWay{}, errors.Wrap(err, "could not get other ways")
	}
	for i := 0; i < ways.Len(); i++ {
		way := ways.At(i)
		onWay, err := OnWay(way, pos)
		logde(errors.Wrap(err, "Could not check if on way"))
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

	return CurrentWay{}, errors.New("could not find a current way")
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
		return matchingWays, errors.Wrap(err, "could not read ways from offline")
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
			return matchingWays, errors.Wrap(err, "could not read nodes from way")
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
		logde(errors.Wrap(err, "could not read next way nodes"))
		return true
	}

	lastNode := nodes.At(nodes.Len() - 1)
	if lastNode.Latitude() == matchNode.Latitude() && lastNode.Longitude() == matchNode.Longitude() {
		return false
	}

	return true
}

func NextWay(way Way, offline Offline, isForward bool) (NextWayResult, error) {
	nodes, err := way.Nodes()
	if err != nil {
		return NextWayResult{}, errors.Wrap(err, "could not read way nodes")
	}
	if !way.HasNodes() || nodes.Len() == 0 {
		return NextWayResult{}, nil
	}

	var matchNode Coordinates
	var matchBearingNode Coordinates
	if isForward {
		matchNode = nodes.At(nodes.Len() - 1)
		matchBearingNode = nodes.At(nodes.Len() - 2)
	} else {
		matchNode = nodes.At(0)
		matchBearingNode = nodes.At(1)
	}

	if !PointInBox(matchNode.Latitude(), matchNode.Longitude(), offline.MinLat(), offline.MinLon(), offline.MaxLat(), offline.MaxLon()) {
		return NextWayResult{}, nil
	}

	matchingWays, err := MatchingWays(way, offline, matchNode)
	if err != nil {
		return NextWayResult{StartPosition: matchNode}, errors.Wrap(err, "could not check for next ways")
	}

	if len(matchingWays) == 0 {
		return NextWayResult{StartPosition: matchNode}, nil
	}

	// first return if one of the next connecting ways has the same name
	name, _ := way.Name()
	if len(name) > 0 {
		for _, mWay := range matchingWays {
			mName, err := mWay.Name()
			if err != nil {
				return NextWayResult{StartPosition: matchNode}, errors.Wrap(err, "could not read way name")
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
	ref, _ := way.Ref()
	if len(ref) > 0 {
		for _, mWay := range matchingWays {
			mRef, err := mWay.Ref()
			if err != nil {
				return NextWayResult{StartPosition: matchNode}, errors.Wrap(err, "could not read way ref")
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

	// third return the next connecting way with the least curvature
	minCurvWay := matchingWays[0]
	minCurv := float64(100)
	for _, mWay := range matchingWays {
		nodes, err := mWay.Nodes()
		if err != nil {
			continue
		}

		var bearingNode Coordinates
		if matchNode.Latitude() == nodes.At(0).Latitude() && matchNode.Longitude() == nodes.At(0).Longitude() {
			bearingNode = nodes.At(1)
		} else {
			bearingNode = nodes.At(nodes.Len() - 2)
		}

		mCurv, _ := GetCurvature(matchBearingNode.Latitude(), matchBearingNode.Longitude(), matchNode.Latitude(), matchNode.Longitude(), bearingNode.Latitude(), bearingNode.Longitude())
		mCurv = math.Abs(mCurv)

		if mCurv < minCurv {
			minCurv = mCurv
			minCurvWay = mWay
		}
	}

	nextIsForward := NextIsForward(minCurvWay, matchNode)
	start, end := GetWayStartEnd(minCurvWay, nextIsForward)
	return NextWayResult{
		Way:           minCurvWay,
		StartPosition: start,
		EndPosition:   end,
		IsForward:     nextIsForward,
	}, nil
}

func DistanceToEndOfWay(pos Position, way Way, isForward bool) (float64, error) {
	distanceResult, err := DistanceToWay(pos, way)
	if err != nil {
		return 0, err
	}
	lat := distanceResult.LineEnd.Latitude()
	lon := distanceResult.LineEnd.Longitude()
	dist := DistanceToPoint(pos.Latitude*TO_RADIANS, pos.Longitude*TO_RADIANS, lat*TO_RADIANS, lon*TO_RADIANS)
	stopFiltering := false
	nodes, err := way.Nodes()
	if err != nil {
		return 0, err
	}
	for i := 0; i < nodes.Len(); i++ {
		index := i
		if !isForward {
			index = nodes.Len() - 1 - i
		}
		node := nodes.At(index)
		nLat := node.Latitude()
		nLon := node.Longitude()
		if node.Latitude() == lat && node.Longitude() == lon && !stopFiltering {
			stopFiltering = true
		}
		if !stopFiltering {
			continue
		}
		dist += DistanceToPoint(lat*TO_RADIANS, lon*TO_RADIANS, nLat*TO_RADIANS, nLon*TO_RADIANS)
		lat = nLat
		lon = nLon
	}
	return dist, nil
}

func NextWays(pos Position, currentWay CurrentWay, offline Offline, isForward bool) ([]NextWayResult, error) {
	nextWays := []NextWayResult{}
	dist := 0.0
	wayIdx := currentWay.Way
	forward := isForward
	startPos := pos
	for dist < float64(MIN_WAY_DIST) {
		d, err := DistanceToEndOfWay(startPos, wayIdx, forward)
		if err != nil || d <= 0 {
			break
		}
		dist += d
		nw, err := NextWay(wayIdx, offline, forward)
		if err != nil {
			break
		}
		nextWays = append(nextWays, nw)
		wayIdx = nw.Way
		startPos = Position{
			Latitude:  nw.StartPosition.Latitude(),
			Longitude: nw.StartPosition.Longitude(),
		}
		forward = nw.IsForward
	}

	if len(nextWays) == 0 {
		nextWay, err := NextWay(currentWay.Way, offline, isForward)
		if err != nil {
			return []NextWayResult{}, err
		}
		nextWays = append(nextWays, nextWay)
	}

	return nextWays, nil
}
