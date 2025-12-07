package main

import (
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cereal/car"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/maps"
	m "pfeifer.dev/mapd/math"
	ms "pfeifer.dev/mapd/settings"
)

type State struct {
	Publisher                 *cereal.Publisher[custom.MapdOut]
	Data                      maps.Offline
	Car                       CarState
	CurrentWay                CurrentWay
	SpeedLimit                SpeedLimitState
	NextWays                  []maps.NextWayResult
	Position                  m.Position
	Curvatures                []m.Curvature
	TargetVelocities          []Velocity
	DistanceSinceLastPosition float32
	VisionCurveSpeed          float32
	MapCurveSpeed             float32
	VisionCurveMA             m.MovingAverage
	NextAdvisorySpeed         Upcoming[float32]
	NextHazard                Upcoming[string]
}

func (s *State) Init() {
	s.Car.Init()
	s.VisionCurveMA.Init(20)
	s.NextHazard = NewUpcoming(10, "", checkWayForHazardChange)
	s.NextAdvisorySpeed = NewUpcoming(10, 0, checkWayForAdvisorySpeedChange)
	s.SpeedLimit.Init()
}


func (s *State) SuggestedSpeed() float32 {
	suggestedSpeed := min(s.Car.VCruise*ms.KPH_TO_MS, ms.MAX_OP_SPEED)

	if ms.Settings.SpeedLimitControlEnabled || ms.Settings.ExternalSpeedLimitControlEnabled {
		slSuggestedSpeed := s.SpeedLimit.SpeedLimitFinalSuggestion(s.Car.EnableSpeedActive, s.Car.SetSpeedChanging, s.Car.VEgo)
		if suggestedSpeed > slSuggestedSpeed && slSuggestedSpeed > 0 {
			suggestedSpeed = slSuggestedSpeed
		}
	}
	if ms.Settings.VisionCurveSpeedControlEnabled && s.VisionCurveSpeed > 0 && (s.VisionCurveSpeed < suggestedSpeed || suggestedSpeed == 0) && (!ms.Settings.VisionCurveUseEnableSpeed || s.Car.EnableSpeedActive) {
		suggestedSpeed = s.VisionCurveSpeed
	}
	if ms.Settings.MapCurveSpeedControlEnabled && s.MapCurveSpeed > 0 && (s.MapCurveSpeed < suggestedSpeed || suggestedSpeed == 0) && (!ms.Settings.MapCurveUseEnableSpeed || s.Car.EnableSpeedActive) {
		suggestedSpeed = s.MapCurveSpeed
	}
	if suggestedSpeed < 0 {
		suggestedSpeed = 0
	}
	return suggestedSpeed
}

func (s *State) UpdateCarState(carData car.CarState) {
	s.Car.Update(carData)
	s.DistanceSinceLastPosition += float32(s.Car.UpdateTime.DiffMA.Estimate) * s.Car.VEgo
	s.SpeedLimit.NextLimit.Update(s)
	s.NextAdvisorySpeed.Update(s)
	s.NextHazard.Update(s)
	s.SpeedLimit.Update(s.CurrentWay, s.Car)
}

func (s *State) Send() error {
	msg, output := s.Publisher.NewMessage(true)

	name := s.CurrentWay.Way.WayName()
	output.SetWayName(name)

	ref := s.CurrentWay.Way.WayRef()
	output.SetWayRef(ref)

	output.SetRoadName(s.CurrentWay.Way.Name())

	maxSpeed := s.CurrentWay.MaxSpeed()
	output.SetSpeedLimit(float32(maxSpeed))

	output.SetSpeedLimitSuggestedSpeed(s.SpeedLimit.Suggestion.Value)

	output.SetNextSpeedLimit(s.SpeedLimit.NextLimit.Value)
	output.SetNextSpeedLimitDistance(s.SpeedLimit.NextLimit.Distance)

	hazard := s.CurrentWay.Way.Hazard()
	output.SetHazard(hazard)

	output.SetNextHazard(s.NextHazard.Value)
	output.SetNextHazardDistance(s.NextHazard.Distance)

	advisorySpeed := s.CurrentWay.Way.AdvisorySpeed()
	output.SetAdvisorySpeed(float32(advisorySpeed))

	output.SetNextAdvisorySpeed(s.NextAdvisorySpeed.Value)
	output.SetNextHazardDistance(s.NextAdvisorySpeed.Distance)

	oneWay := s.CurrentWay.Way.OneWay()
	output.SetOneWay(oneWay)

	lanes := s.CurrentWay.Way.Lanes()
	output.SetLanes(uint8(lanes))

	output.SetTileLoaded(s.Data.Loaded)

	output.SetRoadContext(custom.RoadContext(s.CurrentWay.Way.Context()))
	output.SetEstimatedRoadWidth(s.CurrentWay.Way.Width())
	output.SetVisionCurveSpeed(s.VisionCurveSpeed)
	output.SetMapCurveSpeed(s.MapCurveSpeed)

	output.SetSuggestedSpeed(s.SuggestedSpeed())
	output.SetDistanceFromWayCenter(float32(s.CurrentWay.OnWay.Distance.Distance))

	output.SetWaySelectionType(s.CurrentWay.SelectionType)

	return s.Publisher.Send(msg)
}
