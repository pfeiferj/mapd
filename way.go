package main

import (
	"cmp"
	"errors"
	"math"
	"slices"
)

func OnWay(way Way, lat float64, lon float64) (bool, Coordinates, Coordinates) {

	if lat < way.MaxLat() && lat > way.MinLat() && lon < way.MaxLon() && lon > way.MinLon() {
		d, nodeStart, nodeEnd := DistanceToWay(lat, lon, way)
		road_width_estimate := 4 * LANE_WIDTH
		max_dist := 5 + road_width_estimate

		if d < max_dist {
			return true, nodeStart, nodeEnd
		}
	}
	return false, Coordinates{}, Coordinates{}
}

func DistanceToWay(lat float64, lon float64, way Way) (float64, Coordinates, Coordinates) {
	minDistance := math.MaxFloat64
	nodes, err := way.Nodes()
	check(err)
	if nodes.Len() < 2 {
		return minDistance, Coordinates{}, Coordinates{}
	}

	var minNodeStart Coordinates
	var minNodeEnd Coordinates

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
	return minDistance, minNodeStart, minNodeEnd
}

type CurrentWay struct {
	Way       Way
	StartNode Coordinates
	EndNode   Coordinates
}

func GetCurrentWay(state *State, lat float64, lon float64) (CurrentWay, error) {
	if state.Way.Way.HasNodes() {
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
	ways, err := state.Result.Ways()
	check(err)
	for i := 0; i < ways.Len(); i++ {
		way := ways.At(i)
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

func MatchingWays(state *State) ([]Way, Coordinates) {
	matchingWays := []Way{}
	nodes, err := state.Way.Way.Nodes()
	check(err)
	if !state.Way.Way.HasNodes() || nodes.Len() == 0 {
		return matchingWays, Coordinates{}
	}

	wayBearing := Bearing(state.Way.StartNode.Latitude(), state.Way.StartNode.Longitude(), state.Way.EndNode.Latitude(), state.Way.EndNode.Longitude())
	bearingDelta := math.Abs((state.Position.Bearing * TO_RADIANS) - wayBearing)
	isForward := math.Cos(bearingDelta) >= 0
	var matchNode Coordinates
	if isForward {
		matchNode = nodes.At(nodes.Len() - 1)
		check(err)
	} else {
		matchNode = nodes.At(0)
	}

	ways, err := state.Result.Ways()
	check(err)
	for i := 0; i < ways.Len(); i++ {
		w := ways.At(i)
		if !w.HasNodes() {
			continue
		}
		wNodes, err := w.Nodes()
		check(err)
		fNode := wNodes.At(0)
		lNode := wNodes.At(nodes.Len() - 1)
		if fNode == matchNode || lNode == matchNode {
			matchingWays = append(matchingWays, w)
		}
	}

	name, _ := state.Way.Way.Name()
	ref, _ := state.Way.Way.Ref()
	sortMatchingWays := func(a, b Way) int {
		aVal := float64(1000)
		bVal := float64(1000)
		if len(name) > 0 {
			an, _ := a.Name()
			bn, _ := b.Name()
			if an == name {
				aVal = -1000
			}
			if bn == name {
				bVal = -1000
			}
		} else if len(ref) > 0 {
			ar, _ := a.Ref()
			br, _ := b.Ref()
			if ar == ref {
				aVal = -1000
			}
			if br == name {
				bVal = -1000
			}
		} else {
			var aBearingNode Coordinates
			aNodes, err := a.Nodes()
			check(err)
			if matchNode == aNodes.At(0) {
				aBearingNode = aNodes.At(1)
			} else {
				aBearingNode = aNodes.At(aNodes.Len() - 2)
			}
			aBearing := Bearing(matchNode.Latitude(), matchNode.Longitude(), aBearingNode.Latitude(), aBearingNode.Longitude())
			aVal = math.Abs((state.Position.Bearing * TO_RADIANS) - aBearing)

			var bBearingNode Coordinates
			bNodes, err := b.Nodes()
			check(err)
			if matchNode == bNodes.At(0) {
				bBearingNode = bNodes.At(1)
			} else {
				bBearingNode = bNodes.At(bNodes.Len() - 2)
			}
			bBearing := Bearing(matchNode.Latitude(), matchNode.Longitude(), bBearingNode.Latitude(), bBearingNode.Longitude())
			bVal = math.Abs((state.Position.Bearing * TO_RADIANS) - bBearing)
		}

		return cmp.Compare(aVal, bVal)
	}
	slices.SortFunc(matchingWays, sortMatchingWays)
	return matchingWays, matchNode
}
