package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"
)

type State struct {
	Result       Offline
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
	generatePtr := flag.Bool("generate", false, "Triggers a generation of map data from 'map.osm.pbf'")
	flag.Parse()
	if *generatePtr {
		GenerateOffline()
		return
	}
	EnsureParamDirectories()
	lastSpeedLimit := float64(0)
	lastAdvisoryLimit := float64(0)
	lastNextSpeedLimit := float64(0)
	speedLimit := float64(0)
	advisoryLimit := float64(0)
	state := State{}

	var pos Position

	coordinates, _ := GetParam(LAST_GPS_POSITION_PERSIST)
	err := json.Unmarshal(coordinates, &pos)
	loge(err)
	state.Result, err = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
	if err != nil {
		loge(err)
	}

	for {
		DownloadIfTriggered()
		coordinates, err := GetParam(LAST_GPS_POSITION)
		loge(err)
		err = json.Unmarshal(coordinates, &pos)
		loge(err)

		state.Position = pos

		if !PointInBox(pos.Latitude, pos.Longitude, float64(state.Result.MinLat()), float64(state.Result.MinLon()), float64(state.Result.MaxLat()), float64(state.Result.MaxLon())) {
			state.MatchingWays = []Way{}
			state.MatchNode = Coordinates{}
			state.Way = CurrentWay{}
			state.Result = Offline{}
			res, err := FindWaysAroundLocation(pos.Latitude, pos.Longitude)
			loge(err)
			if err == nil {
				state.Result = res
			}
		}
		way, err := GetCurrentWay(&state, pos.Latitude, pos.Longitude)
		if err == nil {
			state.Way.StartNode = way.StartNode
			state.Way.EndNode = way.EndNode
			if way.Way.MinLat() != state.Way.Way.MinLat() && way.Way.MinLon() != state.Way.Way.MinLon() && way.Way.MaxLat() != state.Way.Way.MaxLat() && way.Way.MaxLon() != state.Way.Way.MaxLon() {
				state.Way = way
				state.MatchingWays, state.MatchNode, err = MatchingWays(&state)
				loge(err)
				err := PutParam(ROAD_NAME, []byte(RoadName(way.Way)))
				loge(err)
			}
			speedLimit = way.Way.MaxSpeed()
			advisoryLimit = way.Way.AdvisorySpeed()
		} else {
			speedLimit = 0
			advisoryLimit = 0
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
						lastSpeedLimit = 0
						loge(err)
					}
				}
			}
		}

		if speedLimit != lastSpeedLimit {
			lastSpeedLimit = speedLimit
			err := PutParam(MAP_SPEED_LIMIT, []byte(fmt.Sprintf("%f", speedLimit)))
			if err != nil {
				lastSpeedLimit = 0
				loge(err)
			}
		}
		if advisoryLimit != lastAdvisoryLimit {
			lastAdvisoryLimit = advisoryLimit
			err := PutParam(MAP_ADVISORY_LIMIT, []byte(fmt.Sprintf("%f", advisoryLimit)))
			if err != nil {
				lastAdvisoryLimit = 0
				loge(err)

			}
		}
		time.Sleep(1 * time.Second)
	}
}
