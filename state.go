package main

import (
	"time"
)

type State struct {
	Data                   []uint8
	CurrentWay             CurrentWay
	NextWays               []NextWayResult
	Position               Position
	LastPosition           Position
	StableWayCounter       int
	LastGPSUpdate          time.Time
	LastWayChange          time.Time
	LastSpeedLimitDistance float64
	LastSpeedLimitValue    float64
	LastSpeedLimitWayName  string
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
	Distance   float64 `json:"distance"`
}

type AdvisoryLimit struct {
	StartLatitude  float64 `json:"start_latitude"`
	StartLongitude float64 `json:"start_longitude"`
	EndLatitude    float64 `json:"end_latitude"`
	EndLongitude   float64 `json:"end_longitude"`
	Speedlimit     float64 `json:"speedlimit"`
}

type Hazard struct {
	StartLatitude  float64 `json:"start_latitude"`
	StartLongitude float64 `json:"start_longitude"`
	EndLatitude    float64 `json:"end_latitude"`
	EndLongitude   float64 `json:"end_longitude"`
	Hazard         string  `json:"hazard"`
}
