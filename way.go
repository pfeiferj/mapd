package main

import (
	"errors"
	"github.com/serjvanilla/go-overpass"
	"math"
	"strconv"
)

func OnWay(way *overpass.Way, lat float64, lon float64) (bool, *overpass.Node, *overpass.Node) {
	box := Bounds(way.Nodes)

	if lat < box.MaxLat && lat > box.MinLat && lon < box.MaxLon && lon > box.MinLon {
		lanes := float64(2)
		if lanesStr, ok := way.Tags["lanes"]; ok {
			parsedLanes, err := strconv.ParseUint(lanesStr, 10, 64)
			if err == nil {
				lanes = float64(parsedLanes)
			}
		}

		d, nodeStart, nodeEnd := DistanceToWay(lat, lon, way)
		road_width_estimate := lanes * LANE_WIDTH
		max_dist := 5 + road_width_estimate

		if d < max_dist {
			return true, nodeStart, nodeEnd
		}
	}
	return false, nil, nil
}

func DistanceToWay(lat float64, lon float64, way *overpass.Way) (float64, *overpass.Node, *overpass.Node) {
	minDistance := math.MaxFloat64
	if len(way.Nodes) < 2 {
		return minDistance, nil, nil
	}

	var minNodeStart *overpass.Node
	var minNodeEnd *overpass.Node

	latRad := lat * TO_RADIANS
	lonRad := lon * TO_RADIANS
	for i := 0; i < len(way.Nodes)-1; i++ {
		nodeStart := way.Nodes[i]
		nodeEnd := way.Nodes[i+1]
		lineLat, lineLon := PointOnLine(nodeStart.Lat, nodeStart.Lon, nodeEnd.Lat, nodeEnd.Lon, lat, lon)
		distance := DistanceToPoint(latRad, lonRad, lineLat*TO_RADIANS, lineLon*TO_RADIANS)
		if distance < minDistance {
			minDistance = distance
			minNodeStart = nodeStart
			minNodeEnd = nodeEnd
		}
	}
	return minDistance, minNodeStart, minNodeEnd
}

type CurrentWay struct {
	Way       *overpass.Way
	StartNode *overpass.Node
	EndNode   *overpass.Node
}

func GetCurrentWay(state *State, lat float64, lon float64) (CurrentWay, error) {
	// check current way first
	if state.Way.Way != nil {
		onWay, nodeStart, nodeEnd := OnWay(state.Way.Way, lat, lon)
		if onWay {
			return CurrentWay{
				Way:       state.Way.Way,
				StartNode: nodeStart,
				EndNode:   nodeEnd,
			}, nil
		}
	}

	// check ways that have the same name/ref
	for _, way := range state.MatchingWays {
		onWay, nodeStart, nodeEnd := OnWay(way, lat, lon)
		if onWay {
			return CurrentWay{
				Way:       way,
				StartNode: nodeStart,
				EndNode:   nodeEnd,
			}, nil
		}
	}

	// finally check all other ways
	for _, way := range state.Result.Ways {
		onWay, nodeStart, nodeEnd := OnWay(way, lat, lon)
		if onWay {
			return CurrentWay{
				Way:       way,
				StartNode: nodeStart,
				EndNode:   nodeEnd,
			}, nil
		}
	}

	return CurrentWay{}, errors.New("Could not find way")
}

func MatchingWays(way *overpass.Way, ways map[int64]*overpass.Way) []*overpass.Way {
	matchingWays := []*overpass.Way{}
	if way == nil {
		return matchingWays
	}
	name, ok := way.Meta.Tags["name"]
	if ok {
		for _, w := range ways {
			n, k := w.Meta.Tags["name"]
			if k && n == name {
				matchingWays = append(matchingWays, w)
			}
		}
	}
	ref, ok := way.Meta.Tags["ref"]
	if ok {
		for _, w := range ways {
			r, k := w.Meta.Tags["ref"]
			if k && r == ref {
				matchingWays = append(matchingWays, w)
			}
		}
	}
	return matchingWays
}

func NextWay(state *State) (*overpass.Way, *overpass.Node) {
	wayBearing := Bearing(state.Way.StartNode.Lat, state.Way.StartNode.Lon, state.Way.EndNode.Lat, state.Way.EndNode.Lon)
	bearingDelta := math.Abs((state.Position.Bearing * TO_RADIANS) - wayBearing)
	isForward := math.Cos(bearingDelta) >= 0
	var matchNode *overpass.Node
	if isForward {
		matchNode = state.Way.Way.Nodes[len(state.Way.Way.Nodes)-1]
	} else {
		matchNode = state.Way.Way.Nodes[0]
	}

	for _, way := range state.MatchingWays {
		if isForward {
			if matchNode == way.Nodes[0] {
				return way, matchNode
			}
		} else {
			if matchNode == way.Nodes[len(way.Nodes)-1] {
				return way, matchNode
			}
		}
	}

	possibleWays := make([]*overpass.Way, 0)
	for _, way := range state.Result.Ways {
		if isForward {
			if matchNode == way.Nodes[0] {
				possibleWays = append(possibleWays, way)
			}
		} else {
			if matchNode == way.Nodes[len(way.Nodes)-1] {
				possibleWays = append(possibleWays, way)
			}
		}
	}

	if len(possibleWays) == 0 {
		return nil, nil
	}

	smallestDeltaWay := possibleWays[0]
	smallestDelta := float64(2 * math.Pi)
	for _, way := range possibleWays {
		if len(way.Nodes) < 2 {
			continue
		}
		if isForward {
			wayBearing := Bearing(matchNode.Lat, matchNode.Lon, way.Nodes[1].Lat, way.Nodes[1].Lon)
			delta := math.Abs((state.Position.Bearing * TO_RADIANS) - wayBearing)
			if delta < smallestDelta {
				smallestDelta = delta
				smallestDeltaWay = way
			}
		} else {
			wayBearing := Bearing(matchNode.Lat, matchNode.Lon, way.Nodes[len(way.Nodes)-2].Lat, way.Nodes[len(way.Nodes)-2].Lon)
			delta := math.Abs((state.Position.Bearing * TO_RADIANS) - wayBearing)
			if delta < smallestDelta {
				smallestDelta = delta
				smallestDeltaWay = way
			}
		}
	}

	return smallestDeltaWay, matchNode
}

func RoadName(way *overpass.Way) string {
	if way == nil {
		return ""
	}
	name, ok := way.Meta.Tags["name"]
	if ok {
		return name
	}
	ref, ok := way.Meta.Tags["ref"]
	if ok {
		return ref
	}
	return ""
}
