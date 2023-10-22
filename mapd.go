package main

import (
	"encoding/json"
	"fmt"
	"github.com/serjvanilla/go-overpass"
	"time"
)

var R = 6373000.0                 // approximate radius of earth in meters
var LANE_WIDTH = 3.7              // meters
var QUERY_RADIUS = float64(3000)  // meters
var PADDING = 10 / R * TO_DEGREES // 10 meters in degrees

type State struct {
	Result       overpass.Result
	Way          CurrentWay
	NextWay      *overpass.Way
	MatchingWays []*overpass.Way
	Position     Position
	Lat          float64
	Lon          float64
	PendingLat   float64
	PendingLon   float64
	Querying     bool
}

type Position struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Bearing   float64 `json:"bearing"`
}

type NextSpeedLimit struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Speedlimit float64 `json:"speedlimit"`
}

func main() {
	EnsureParamDirectories()
	lastSpeedLimit := float64(0)
	speedLimit := float64(0)
	state := State{}

	var pos Position

	resChannel := make(chan overpass.Result)
	errChannel := make(chan error)

	coordinates, _ := GetParam(LAST_GPS_POSITION_PERSIST)
	json.Unmarshal(coordinates, &pos)
	state.PendingLat = pos.Latitude
	state.PendingLon = pos.Longitude

	go AsyncFetchRoadsAroundLocation(resChannel, errChannel, pos.Latitude, pos.Longitude, QUERY_RADIUS)
	state.Querying = true
	for {
		coordinates, _ := GetParam(LAST_GPS_POSITION)
		json.Unmarshal(coordinates, &pos)
		state.Position = pos

		select {
		case res := <-resChannel:
			state.Querying = false
			state.Result = res
			state.Lat = state.PendingLat * TO_RADIANS
			state.Lon = state.PendingLon * TO_RADIANS
		case err := <-errChannel:
			fmt.Println(err)
			state.PendingLat = pos.Latitude
			state.PendingLon = pos.Longitude
			go AsyncFetchRoadsAroundLocation(resChannel, errChannel, pos.Latitude, pos.Longitude, QUERY_RADIUS)
		default:
		}

		d := DistanceToPoint(pos.Latitude*TO_RADIANS, pos.Longitude*TO_RADIANS, state.Lat, state.Lon)
		if !state.Querying && d > 0.7*QUERY_RADIUS {
			state.PendingLat = pos.Latitude
			state.PendingLon = pos.Longitude
			state.Querying = true
			go AsyncFetchRoadsAroundLocation(resChannel, errChannel, pos.Latitude, pos.Longitude, QUERY_RADIUS)
		}
		way, err := GetCurrentWay(&state, pos.Latitude, pos.Longitude)
		state.Way.StartNode = way.StartNode
		state.Way.EndNode = way.EndNode
		if way.Way != state.Way.Way {
			state.Way = way
			state.MatchingWays = MatchingWays(way.Way, state.Result.Ways)
			err := PutParam(ROAD_NAME, []byte(RoadName(way.Way)))
			if err != nil {
				fmt.Println(err)
			}
		}

		if err == nil {
			speedLimit = ParseMaxSpeed(way.Way.Tags["maxspeed"])
		} else {
			speedLimit = 0
		}

		if state.Way.Way != nil {
			nextWay, nextWayNode := NextWay(&state)
			state.NextWay = nextWay
			if state.NextWay != nil {
				nextWaySpeedLimit := ParseMaxSpeed(state.NextWay.Tags["maxspeed"])
				if speedLimit != nextWaySpeedLimit {
					data, _ := json.Marshal(NextSpeedLimit{
						Latitude:   nextWayNode.Lat,
						Longitude:  nextWayNode.Lon,
						Speedlimit: nextWaySpeedLimit,
					})
					err := PutParam(NEXT_MAP_SPEED_LIMIT, data)
					if err != nil {
						fmt.Println(err)
					}
				}
			}
		}

		if speedLimit != lastSpeedLimit {
			lastSpeedLimit = speedLimit
			err := PutParam(MAP_SPEED_LIMIT, []byte(fmt.Sprintf("%f", speedLimit)))
			if err != nil {
				fmt.Println(err)
			}
		}
		time.Sleep(1 * time.Second)
	}
}
