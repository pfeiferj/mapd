package main

import (
	"pfeifer.dev/mapd/cereal/log"
)

type State struct {
	Data                   []uint8
	CurrentWay             CurrentWay
	LastWay                CurrentWay
	NextWays               []NextWayResult
	Location               log.GpsLocationData
	LastLocation           log.GpsLocationData
	StableWayCounter       int
	Curvatures             []Curvature
	TargetVelocities       []Velocity
	MaxSpeed               float64
	LastSpeedLimitDistance float64
	LastSpeedLimitValue    float64
	LastSpeedLimitWayName  string
	NextSpeedLimit         NextSpeedLimit
	VtscSpeed              float32
}
