package main

import (
	"capnproto.org/go/capnp/v3"
	"encoding/json"
	"flag"
	"fmt"
	"time"
)

type State struct {
	Data         []uint8
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
	name, err := way.Name()
	if err == nil {
		if len(name) > 0 {
			return name
		}
	}
	ref, err := way.Ref()
	if err == nil {
		if len(ref) > 0 {
			return ref
		}
	}
	return ""
}

func main() {
	generatePtr := flag.Bool("generate", false, "Triggers a generation of map data from 'map.osm.pbf'")
	minGenLatPtr := flag.Int("minlat", -90, "the minimum latitude to generate")
	minGenLonPtr := flag.Int("minlon", -180, "the minimum longitude to generate")
	maxGenLatPtr := flag.Int("maxlat", -90, "the maximum latitude to generate")
	maxGenLonPtr := flag.Int("maxlon", -180, "the maximum longitude to generate")
	flag.Parse()
	if *generatePtr {
		GenerateOffline(*minGenLatPtr, *minGenLonPtr, *maxGenLatPtr, *maxGenLonPtr)
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
	state.Data, err = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
	loge(err)

	for {
		DownloadIfTriggered()

		msg, err := capnp.UnmarshalPacked(state.Data)
		loge(err)
		if err == nil {
			offline, err := ReadRootOffline(msg)
			loge(err)
			state.Result = offline
		}
		coordinates, err := GetParam(LAST_GPS_POSITION)
		if err != nil {
			loge(err)
			continue
		}
		err = json.Unmarshal(coordinates, &pos)
		if err != nil {
			loge(err)
			continue
		}
		state.Position = pos

		if !PointInBox(pos.Latitude, pos.Longitude, state.Result.MinLat(), state.Result.MinLon(), state.Result.MaxLat(), state.Result.MaxLon()) {
			state.MatchingWays = []Way{}
			state.MatchNode = Coordinates{}
			state.Way = CurrentWay{}
			state.Result = Offline{}
			state.NextWay = Way{}
			state.Data, err = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
			loge(err)
		}
		way, err := GetCurrentWay(&state, pos.Latitude, pos.Longitude)
		if err == nil {
			state.Way.StartNode = way.StartNode
			state.Way.EndNode = way.EndNode
			state.Way = way
			state.MatchingWays, state.MatchNode, err = MatchingWays(&state)
			loge(err)
			err := PutParam(ROAD_NAME, []byte(RoadName(way.Way)))
			loge(err)
			speedLimit = way.Way.MaxSpeed()
			advisoryLimit = way.Way.AdvisorySpeed()
		} else {
			speedLimit = 0
			advisoryLimit = 0
		}

		if len(state.MatchingWays) > 0 {
			state.NextWay = state.MatchingWays[0]
			if state.NextWay.HasNodes() {
				nextSpeedLimit := state.NextWay.MaxSpeed()
				if nextSpeedLimit != lastNextSpeedLimit {
					lastNextSpeedLimit = nextSpeedLimit
					data, err := json.Marshal(NextSpeedLimit{
						Latitude:   state.MatchNode.Latitude(),
						Longitude:  state.MatchNode.Longitude(),
						Speedlimit: nextSpeedLimit,
					})

					loge(err)
					if err == nil {
						err := PutParam(NEXT_MAP_SPEED_LIMIT, data)
						if err != nil {
							lastNextSpeedLimit = 0
							loge(err)
						}
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
