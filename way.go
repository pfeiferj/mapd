package main

import (
	"errors"
	"github.com/serjvanilla/go-overpass"
	"math"
	"strconv"
)

func OnWay(way *overpass.Way, lat float64, lon float64) bool {
	box := Bounds(way.Nodes)

	if lat < box.MaxLat && lat > box.MinLat && lon < box.MaxLon && lon > box.MinLon {
		lanes := float64(2)
		if lanesStr, ok := way.Tags["lanes"]; ok {
			parsedLanes, err := strconv.ParseUint(lanesStr, 10, 64)
			if err == nil {
				lanes = float64(parsedLanes)
			}
		}

		d := DistanceToWay(lat, lon, way)
		road_width_estimate := lanes * LANE_WIDTH
		max_dist := 5 + road_width_estimate

		if d < max_dist {
			return true
		}
	}
	return false
}

func DistanceToWay(lat float64, lon float64, way *overpass.Way) float64 {
	minDistance := math.MaxFloat64
	if len(way.Nodes) < 2 {
		return minDistance
	}

	latRad := lat * math.Pi / 180
	lonRad := lon * math.Pi / 180
	for i := 0; i < len(way.Nodes)-1; i++ {
		nodeStart := way.Nodes[i]
		nodeEnd := way.Nodes[i+1]
		lineLat, lineLon := PointOnLine(nodeStart.Lat, nodeStart.Lon, nodeEnd.Lat, nodeEnd.Lon, lat, lon)
		distance := DistanceToPoint(latRad, lonRad, lineLat*math.Pi/180, lineLon*math.Pi/180)
		if distance < minDistance {
			minDistance = distance
		}
	}
	return minDistance
}

func GetCurrentWay(cache *Cache, lat float64, lon float64) (*overpass.Way, error) {
	// check current way first
	if cache.Way != nil {
		if OnWay(cache.Way, lat, lon) {
			return cache.Way, nil
		}
	}

	// check ways that have the same name/ref
	for _, way := range cache.MatchingWays {
		if OnWay(way, lat, lon) {
			return way, nil
		}
	}

	// finally check all other ways
	for _, way := range cache.Result.Ways {
		if OnWay(way, lat, lon) {
			return way, nil
		}
	}

	return nil, errors.New("Could not find way")
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
