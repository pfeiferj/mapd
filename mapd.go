package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/serjvanilla/go-overpass"
	"math"
	"strconv"
	"strings"
	"time"
)

var R = 6373000.0                      // approximate radius of earth in meters
var LANE_WIDTH = 3.7                   // meters
var QUERY_RADIUS = float64(3000)       // meters
var PADDING = 10 / R * (180 / math.Pi) // 10 meters in degrees

type Cache struct {
	Result       overpass.Result
	Way          *overpass.Way
	MatchingWays []*overpass.Way
	Lat          float64
	Lon          float64
}

type Position struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func FetchRoadsAroundLocation(lat float64, lon float64, radius float64) (overpass.Result, error) {
	client := overpass.New()
	bbox_angle := (radius / R) * (180 / math.Pi)
	bbox_str := fmt.Sprintf("%f,%f,%f,%f", lat-bbox_angle, lon-bbox_angle, lat+bbox_angle, lon+bbox_angle)
	query_fmt := "[out:json];way(%s)\n" +
		"[highway]\n" +
		"[highway!~\"^(footway|path|corridor|bridleway|steps|cycleway|construction|bus_guideway|escape|service|track)$\"];\n" +
		"(._;>;);\n" +
		"out;\n" +
		"is_in(%f,%f);area._[admin_level~\"[24]\"];\n" +
		"convert area ::id = id(), admin_level = t['admin_level'],\n" +
		"name = t['name'], \"ISO3166-1:alpha2\" = t['ISO3166-1:alpha2'];out;"
	query := fmt.Sprintf(query_fmt, bbox_str, lat, lon)
	result, err := client.Query(query)
	return result, err
}

func ParseMaxSpeed(maxspeed string) float64 {
	splitSpeed := strings.Split(maxspeed, " ")
	if len(splitSpeed) != 2 {
		return 0
	}
	numeric, err := strconv.ParseUint(splitSpeed[0], 10, 64)
	if err != nil {
		return 0
	}

	if splitSpeed[1] == "kph" || splitSpeed[1] == "km/h" || splitSpeed[1] == "kmh" {
		return 0.277778 * float64(numeric)
	} else if splitSpeed[1] == "mph" {
		return 0.44704 * float64(numeric)
	} else if splitSpeed[1] == "knots" {
		return 0.514444 * float64(numeric)
	}

	return 0
}

type Box struct {
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
}

func Bounds(nodes []*overpass.Node) Box {
	box := Box{
		MinLat: 90,
		MinLon: 180,
		MaxLat: -90,
		MaxLon: -180,
	}

	for _, node := range nodes {
		if node.Lat > box.MaxLat {
			box.MaxLat = node.Lat
		}
		if node.Lat < box.MinLat {
			box.MinLat = node.Lat
		}
		if node.Lon > box.MaxLon {
			box.MaxLon = node.Lon
		}
		if node.Lon < box.MinLon {
			box.MinLon = node.Lon
		}
	}

	box.MaxLat = box.MaxLat + PADDING
	box.MaxLon = box.MaxLon + PADDING
	box.MinLat = box.MinLat - PADDING
	box.MinLon = box.MinLon - PADDING
	return box
}

func Dot(ax float64, ay float64, bx float64, by float64) float64 {
	return (ax * bx) + (ay * by)
}

func PointOnLine(startLat float64, startLon float64, endLat float64, endLon float64, lat float64, lon float64) (float64, float64) {
	aplat := lat - startLat
	aplon := lon - startLon

	ablat := endLat - startLat
	ablon := endLon - startLon

	t := Dot(aplat, aplon, ablat, ablon) / Dot(ablat, ablon, ablat, ablon)

	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	latitude := startLat + t*ablat
	longitude := startLon + t*ablon

	return latitude, longitude
}

func DistanceToPoint(ax float64, ay float64, bx float64, by float64) float64 {
	a := math.Sin((bx-ax)/2)*math.Sin((bx-ax)/2) + math.Cos(ax)*math.Cos(bx)*math.Sin((by-ay)/2)*math.Sin((by-ay)/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c // in metres
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

func AsyncFetchRoadsAroundLocation(resChannel chan overpass.Result, errChannel chan error, lat float64, lon float64, radius float64) {
	res, err := FetchRoadsAroundLocation(lat, lon, radius)
	if err != nil {
		errChannel <- err
		return
	}
	resChannel <- res
}

func main() {
	EnsureParamDirectories()
	lastSpeedLimit := float64(0)
	speedLimit := float64(0)
	cache := Cache{}
	var pos Position

	resChannel := make(chan overpass.Result)
	errChannel := make(chan error)

	coordinates, _ := GetParam(ParamPath("LastGPSPosition", false))
	json.Unmarshal(coordinates, &pos)
	queryLat := pos.Latitude
	queryLon := pos.Longitude

	go AsyncFetchRoadsAroundLocation(resChannel, errChannel, pos.Latitude, pos.Longitude, QUERY_RADIUS)
	querying := true
	for {
		coordinates, _ := GetParam(ParamPath("LastGPSPosition", true))
		json.Unmarshal(coordinates, &pos)

		select {
		case res := <-resChannel:
			querying = false
			cache.Result = res
			cache.Lat = queryLat * math.Pi / 180
			cache.Lon = queryLon * math.Pi / 180
		case err := <-errChannel:
			fmt.Println(err)
			queryLat = pos.Latitude
			queryLon = pos.Longitude
			go AsyncFetchRoadsAroundLocation(resChannel, errChannel, pos.Latitude, pos.Longitude, QUERY_RADIUS)
		default:
		}

		d := DistanceToPoint(pos.Latitude*math.Pi/180, pos.Longitude*math.Pi/180, cache.Lat, cache.Lon)
		if !querying && d > 0.7*QUERY_RADIUS {
			queryLat = pos.Latitude
			queryLon = pos.Longitude
			querying = true
			go AsyncFetchRoadsAroundLocation(resChannel, errChannel, pos.Latitude, pos.Longitude, QUERY_RADIUS)
		}
		way, err := GetCurrentWay(&cache, pos.Latitude, pos.Longitude)
		if way != cache.Way {
			cache.Way = way
			cache.MatchingWays = MatchingWays(way, cache.Result.Ways)
		}

		if err == nil {
			speedLimit = ParseMaxSpeed(way.Tags["maxspeed"])
		} else {
			speedLimit = 0
		}

		if speedLimit != lastSpeedLimit {
			lastSpeedLimit = speedLimit
			err := PutParam(ParamPath("MapSpeedLimit", true), []byte(fmt.Sprintf("%f", speedLimit)))
			if err != nil {
				fmt.Println(err)
			}
		}
		time.Sleep(1 * time.Second)
	}
}
