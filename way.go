package main

import (
	"cmp"
	"errors"
	"math"
	"slices"
	"strconv"

	"github.com/serjvanilla/go-overpass"
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

func MatchingWays(state *State) ([]*overpass.Way, *overpass.Node) {
	matchingWays := []*overpass.Way{}
	if state.Way.Way == nil || len(state.Way.Way.Nodes) == 0 {
		return matchingWays, nil
	}

	wayBearing := Bearing(state.Way.StartNode.Lat, state.Way.StartNode.Lon, state.Way.EndNode.Lat, state.Way.EndNode.Lon)
	bearingDelta := math.Abs((state.Position.Bearing * TO_RADIANS) - wayBearing)
	isForward := math.Cos(bearingDelta) >= 0
	var matchNode *overpass.Node
	if isForward {
		matchNode = state.Way.Way.Nodes[len(state.Way.Way.Nodes)-1]
	} else {
		matchNode = state.Way.Way.Nodes[0]
	}

	for _, w := range state.Result.Ways {
		if len(w.Nodes) == 0 {
			continue
		}
		fNode := w.Nodes[0]
		lNode := w.Nodes[len(w.Nodes)-1]
		if fNode == matchNode || lNode == matchNode {
			matchingWays = append(matchingWays, w)
		}
	}

	name, nameOk := state.Way.Way.Meta.Tags["name"]
	ref, refOk := state.Way.Way.Meta.Tags["ref"]
	sortMatchingWays := func(a, b *overpass.Way) int {
		aVal := float64(1000)
		bVal := float64(1000)
		if nameOk {
			an := a.Tags["name"]
			bn := b.Tags["name"]
			if an == name {
				aVal = -1000
			}
			if bn == name {
				bVal = -1000
			}
		} else if refOk {
			ar := a.Tags["ref"]
			br := b.Tags["ref"]
			if ar == ref {
				aVal = -1000
			}
			if br == name {
				bVal = -1000
			}
		} else {
			var aBearingNode *overpass.Node
			if matchNode == a.Nodes[0] {
				aBearingNode = a.Nodes[1]
			} else {
				aBearingNode = a.Nodes[len(a.Nodes)-2]
			}
			aBearing := Bearing(matchNode.Lat, matchNode.Lon, aBearingNode.Lat, aBearingNode.Lon)
			aVal = math.Abs((state.Position.Bearing * TO_RADIANS) - aBearing)

			var bBearingNode *overpass.Node
			if matchNode == b.Nodes[0] {
				bBearingNode = b.Nodes[1]
			} else {
				bBearingNode = b.Nodes[len(b.Nodes)-2]
			}
			bBearing := Bearing(matchNode.Lat, matchNode.Lon, bBearingNode.Lat, bBearingNode.Lon)
			bVal = math.Abs((state.Position.Bearing * TO_RADIANS) - bBearing)
		}

		return cmp.Compare(aVal, bVal)
	}
	slices.SortFunc(matchingWays, sortMatchingWays)
	return matchingWays, matchNode
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
