package main

import (
	"encoding/json"
	"fmt"
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
	loge(err)
	state.Result, state.ResultArea, err = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
	loge(err)

	for {
		coordinates, err := GetParam(LAST_GPS_POSITION)
		loge(err)
		err = json.Unmarshal(coordinates, &pos)
		loge(err)

		state.Position = pos

		if !PointInBox(pos.Latitude, pos.Longitude, state.ResultArea.MinLat, state.ResultArea.MinLon, state.ResultArea.MaxLat, state.ResultArea.MaxLon) {
			state.Result, state.ResultArea, err = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
			loge(err)
		}
		way, err := GetCurrentWay(&state, pos.Latitude, pos.Longitude)
		state.Way.StartNode = way.StartNode
		state.Way.EndNode = way.EndNode
		if way.Way != state.Way.Way {
			state.Way = way
			state.MatchingWays, state.MatchNode, err = MatchingWays(&state)
			loge(err)
			err := PutParam(ROAD_NAME, []byte(RoadName(way.Way)))
			loge(err)
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
					loge(err)
				}
			}
		}

		if speedLimit != lastSpeedLimit {
			lastSpeedLimit = speedLimit
			err := PutParam(MAP_SPEED_LIMIT, []byte(fmt.Sprintf("%f", speedLimit)))
			loge(err)
		}
		time.Sleep(1 * time.Second)
	}
}
