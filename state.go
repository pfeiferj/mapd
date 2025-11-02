package main

import (
	"math"

	"capnproto.org/go/capnp/v3"

	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/cereal/car"
	ms "pfeifer.dev/mapd/settings"
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
	CarSetSpeed            float32
	CarVEgo                float32
	CarAEgo                float32
	CurveSpeed             float32
}

func (s *State) checkEnableSpeed() bool {
	if ms.Settings.EnableSpeed == 0 {
		return true
	}
	return math.Abs(float64(s.CarSetSpeed - ms.Settings.EnableSpeed)) < ms.ENABLE_SPEED_RANGE
}

func (s *State) SuggestedSpeed() float32 {
	suggestedSpeed := float32(0)
	if ms.Settings.SpeedLimitControlEnabled && (!ms.Settings.SpeedLimitUseEnableSpeed || s.checkEnableSpeed()){
		suggestedSpeed = float32(s.MaxSpeed)
		if suggestedSpeed == 0 && ms.Settings.HoldLastSeenSpeedLimit {
			suggestedSpeed = float32(s.LastSpeedLimitValue)
		}
		if suggestedSpeed > 0 {
			suggestedSpeed += ms.Settings.SpeedLimitOffset

			offsetNextSpeedLimit := s.NextSpeedLimit.Speedlimit + float64(ms.Settings.SpeedLimitOffset)
			timeToNextSpeedLimit := suggestedSpeed / float32(s.NextSpeedLimit.Distance)
			speedLimitDiff := math.Abs(offsetNextSpeedLimit - float64(suggestedSpeed))
			timeToAdjust := ms.Settings.CurveTargetAccel / float32(speedLimitDiff)
			if timeToAdjust < 1.5 { //deal with infrequent position updates
				timeToAdjust = 1.5
			}

			if s.NextSpeedLimit.Speedlimit > s.MaxSpeed && ms.Settings.SpeedUpForNextSpeedLimit && timeToAdjust > timeToNextSpeedLimit {
				suggestedSpeed = float32(offsetNextSpeedLimit)
			} else if s.NextSpeedLimit.Speedlimit < s.MaxSpeed && ms.Settings.SlowDownForNextSpeedLimit && timeToAdjust > timeToNextSpeedLimit {
				suggestedSpeed = float32(offsetNextSpeedLimit)
			}
		}
	}
	if ms.Settings.VisionCurveSpeedControlEnabled && s.VtscSpeed > 0 && (s.VtscSpeed < suggestedSpeed || suggestedSpeed == 0) && (!ms.Settings.VtscUseEnableSpeed || s.checkEnableSpeed()) {
		suggestedSpeed = s.VtscSpeed
	}
	if ms.Settings.CurveSpeedControlEnabled && s.CurveSpeed > 0 && (s.CurveSpeed < suggestedSpeed || suggestedSpeed == 0) && (!ms.Settings.CurveUseEnableSpeed || s.checkEnableSpeed()) {
		suggestedSpeed = s.CurveSpeed
	}
	if suggestedSpeed < 0 {
		suggestedSpeed = 0
	}
	if suggestedSpeed > 90 * ms.MPH_TO_MS {
		suggestedSpeed = 90 * ms.MPH_TO_MS
	}
	return suggestedSpeed
}

func (s *State) UpdateCarState(carData car.CarState) {
	s.CarSetSpeed = carData.VCruise() * ms.KPH_TO_MS
	s.CarVEgo = carData.VEgo()
	s.CarAEgo = carData.AEgo()
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
	output.SetCurveSpeed(s.CurveSpeed)

	output.SetSuggestedSpeed(s.SuggestedSpeed())

	logOutput(event, output)

	return msg
}
