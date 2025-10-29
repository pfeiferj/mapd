package main

import (
	"capnproto.org/go/capnp/v3"

	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
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

func (s *State) SuggestedSpeed() float32 {
	suggestedSpeed := float32(0)
	if settings.Settings.SpeedLimitControlEnabled {
		suggestedSpeed = float32(s.CurrentWay.Way.MaxSpeed())
		if suggestedSpeed > 0 {
			suggestedSpeed += settings.Settings.SpeedLimitOffset
		}
	}
	if settings.Settings.VisionCurveSpeedControlEnabled && s.VtscSpeed > 0 && (s.VtscSpeed < suggestedSpeed || suggestedSpeed == 0) {
		suggestedSpeed = s.VtscSpeed
	}
	return suggestedSpeed
}

func (s *State) ToMessage() *capnp.Message {
	msg, event, output := newOutput()

	event.SetValid(true)

	name, _ := s.CurrentWay.Way.Name()
	output.SetWayName(name)

	ref, _ := s.CurrentWay.Way.Ref()
	output.SetWayRef(ref)

	roadName := RoadName(s.CurrentWay.Way)
	output.SetRoadName(roadName)

	maxSpeed := s.CurrentWay.Way.MaxSpeed()
	output.SetSpeedLimit(float32(maxSpeed))

	output.SetNextSpeedLimit(float32(s.NextSpeedLimit.Speedlimit))
	output.SetNextSpeedLimitDistance(float32(s.NextSpeedLimit.Distance))

	hazard, _ := s.CurrentWay.Way.Hazard()
	output.SetHazard(hazard)

	advisorySpeed := s.CurrentWay.Way.AdvisorySpeed()
	output.SetAdvisorySpeed(float32(advisorySpeed))

	oneWay := s.CurrentWay.Way.OneWay()
	output.SetOneWay(oneWay)

	lanes := s.CurrentWay.Way.Lanes()
	output.SetLanes(lanes)

	if len(s.Data) > 0 {
		output.SetTileLoaded(true)
	} else {
		output.SetTileLoaded(false)
	}

	output.SetRoadContext(custom.RoadContext(s.CurrentWay.Context))
	output.SetEstimatedRoadWidth(float32(estimateRoadWidth(s.CurrentWay.Way)))
	output.SetVtscSpeed(s.VtscSpeed)

	output.SetSuggestedSpeed(s.SuggestedSpeed())

	logOutput(event, output)

	return msg
}
