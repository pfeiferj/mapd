package main

import (
	"encoding/json"
	"fmt"
	"github.com/serjvanilla/go-overpass"
	"math"
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

func main() {
	EnsureParamDirectories()
	lastSpeedLimit := float64(0)
	speedLimit := float64(0)
	lastRoadName := ""
	roadName := ""
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
			err := PutParam(ParamPath("RoadName", true), []byte(fmt.Sprintf("%s", RoadName(way))))
			if err != nil {
				fmt.Println(err)
			}

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
