package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

var R = 6373000.0                 // approximate radius of earth in meters
var LANE_WIDTH = 3.7              // meters
var QUERY_RADIUS = float64(3000)  // meters
var PADDING = 10 / R * TO_DEGREES // 10 meters in degrees

type State struct {
	Result       Offline
	ResultArea   Area
	Way          CurrentWay
	NextWay      Way
	MatchingWays []Way
	MatchNode    Coordinates
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

func RoadName(way Way) string {
	name, _ := way.Name()
	if len(name) > 0 {
		return name
	}
	ref, _ := way.Ref()
	if len(ref) > 0 {
		return ref
	}
	return ""
}

func main() {
	//GenerateOffline()
	EnsureParamDirectories()
	lastSpeedLimit := float64(0)
	lastNextSpeedLimit := float64(0)
	speedLimit := float64(0)
	state := State{}

	var pos Position

	coordinates, _ := GetParam(LAST_GPS_POSITION_PERSIST)
	err := json.Unmarshal(coordinates, &pos)
	if err != nil {
		log.Printf("%e", err)
	}
	state.PendingLat = pos.Latitude
	state.PendingLon = pos.Longitude
	state.Result, state.ResultArea = FindWaysAroundLocation(pos.Latitude, pos.Longitude)

	for {
		coordinates, err := GetParam(LAST_GPS_POSITION)
		if err != nil {
			log.Printf("%e", err)
		}
		err = json.Unmarshal(coordinates, &pos)
		if err != nil {
			log.Printf("%e", err)
		}

		state.Position = pos

		if !PointInBox(pos.Latitude, pos.Longitude, state.ResultArea.MinLat, state.ResultArea.MinLon, state.ResultArea.MaxLat, state.ResultArea.MaxLon) {
			state.Result, state.ResultArea = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
		}
		way, err := GetCurrentWay(&state, pos.Latitude, pos.Longitude)
		state.Way.StartNode = way.StartNode
		state.Way.EndNode = way.EndNode
		if way.Way != state.Way.Way {
			state.Way = way
			state.MatchingWays, state.MatchNode = MatchingWays(&state)
			err := PutParam(ROAD_NAME, []byte(RoadName(way.Way)))
			if err != nil {
				fmt.Println(err)
			}
		}

		if err == nil {
			speedLimit = way.Way.MaxSpeed()
		} else {
			speedLimit = 0
		}

		if state.Way.Way != (Way{}) && len(state.MatchingWays) > 0 {
			state.NextWay = state.MatchingWays[0]
			if state.NextWay != (Way{}) {
				nextSpeedLimit := state.NextWay.MaxSpeed()
				if nextSpeedLimit != lastNextSpeedLimit {
					lastNextSpeedLimit = nextSpeedLimit
					data, _ := json.Marshal(NextSpeedLimit{
						Latitude:   state.MatchNode.Latitude(),
						Longitude:  state.MatchNode.Longitude(),
						Speedlimit: nextSpeedLimit,
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
