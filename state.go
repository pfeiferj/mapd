package main

import (
	"math"
	"time"

	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cereal/car"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/maps"
	m "pfeifer.dev/mapd/math"
	ms "pfeifer.dev/mapd/settings"
)

type State struct {
	Publisher                 *cereal.Publisher[custom.MapdOut]
	Data                      maps.Offline
	CurrentWay                CurrentWay
	LastWay                   CurrentWay
	NextWays                  []maps.NextWayResult
	Location                  log.GpsLocationData
	LastLocation              log.GpsLocationData
	StableWayCounter          int
	Curvatures                []m.Curvature
	TargetVelocities          []Velocity
	MaxSpeed                  float64
	LastSpeedLimitDistance    float32
	LastSpeedLimitValue       float64
	LastSpeedLimitSuggestion  float32
	AcceptedSpeedLimitValue   float32
	LastSpeedLimitWayName     string
	VisionCurveSpeed          float32
	CarSetSpeed               float32
	SpeedLimitAcceptSetSpeed  float32
	SpeedLimitOverrideSpeed   float32
	TimeLastSetSpeedAdjust    time.Time
	CarVEgo                   float32
	CarAEgo                   float32
	CarVCruise                float32
	LastCarSetSpeed           float32
	GasPressed                bool
	MapCurveSpeed             float32
	VisionCurveMA             m.MovingAverage
	CarStateUpdateTimeMA      m.MovingAverage
	DistanceSinceLastPosition float32
	TimeLastPosition          time.Time
	TimeLastModel             time.Time
	TimeLastCarState          time.Time
	TimeLastSpeedLimitChange  time.Time
	NextSpeedLimit            Upcoming[float32]
	NextAdvisorySpeed         Upcoming[float32]
	NextHazard                Upcoming[string]
}

func (s *State) checkEnableSpeed() bool {
	if ms.Settings.EnableSpeed == 0 {
		return true
	}
	return abs(s.CarSetSpeed-ms.Settings.EnableSpeed) < ms.ENABLE_SPEED_RANGE
}

func (s *State) UpdateSpeedLimitAccept() {
	timeout := ms.Settings.AcceptSpeedLimitTimeout
	if timeout > 0 && time.Since(s.TimeLastSpeedLimitChange) > time.Duration(timeout)*time.Second {
		return
	}
	if ms.Settings.PressGasToAcceptSpeedLimit && s.GasPressed {
		ms.Settings.AcceptSpeedLimit()
	}
	if ms.Settings.AdjustSetSpeedToAcceptSpeedLimit && s.TimeLastSetSpeedAdjust.After(s.TimeLastSpeedLimitChange) {
		if ms.Settings.SpeedLimitAccepted() && s.SpeedLimitAcceptSetSpeed != s.CarSetSpeed {
			ms.Settings.ResetSpeedLimitAccepted()
		}
		if s.SpeedLimitAcceptSetSpeed == 0 {
			s.SpeedLimitAcceptSetSpeed = s.CarSetSpeed
			ms.Settings.AcceptSpeedLimit()
		}
	}
}

func (s *State) SuggestedSpeed() float32 {
	suggestedSpeed := min(s.CarVCruise*ms.KPH_TO_MS, ms.MAX_OP_SPEED)
	setSpeedChanging := time.Since(s.TimeLastSetSpeedAdjust) < 1500*time.Millisecond

	if ms.Settings.SpeedLimitControlEnabled || ms.Settings.ExternalSpeedLimitControlEnabled {
		slSuggestedSpeed := s.SpeedLimit()
		if slSuggestedSpeed != s.LastSpeedLimitSuggestion {
			ms.Settings.ResetSpeedLimitAccepted()
			s.SpeedLimitAcceptSetSpeed = 0
			s.TimeLastSpeedLimitChange = time.Now()
		}
		if ms.Settings.SpeedLimitAccepted() {
			if s.AcceptedSpeedLimitValue != slSuggestedSpeed {
				s.SpeedLimitOverrideSpeed = 0
			}
			s.AcceptedSpeedLimitValue = slSuggestedSpeed
		}
		s.LastSpeedLimitSuggestion = slSuggestedSpeed
		if s.SpeedLimitOverrideSpeed > 0 {
			slSuggestedSpeed = s.SpeedLimitOverrideSpeed
		}
		if suggestedSpeed > slSuggestedSpeed {
			if !ms.Settings.SpeedLimitUseEnableSpeed || s.checkEnableSpeed() {
				suggestedSpeed = s.AcceptedSpeedLimitValue
			} else if setSpeedChanging && ms.Settings.HoldSpeedLimitWhileChangingSetSpeed && s.CarVEgo-1 < slSuggestedSpeed {
				suggestedSpeed = s.AcceptedSpeedLimitValue
			}
		}
	}
	if ms.Settings.VisionCurveSpeedControlEnabled && s.VisionCurveSpeed > 0 && (s.VisionCurveSpeed < suggestedSpeed || suggestedSpeed == 0) && (!ms.Settings.VisionCurveUseEnableSpeed || s.checkEnableSpeed()) {
		suggestedSpeed = s.VisionCurveSpeed
	}
	if ms.Settings.MapCurveSpeedControlEnabled && s.MapCurveSpeed > 0 && (s.MapCurveSpeed < suggestedSpeed || suggestedSpeed == 0) && (!ms.Settings.MapCurveUseEnableSpeed || s.checkEnableSpeed()) {
		suggestedSpeed = s.MapCurveSpeed
	}
	if suggestedSpeed < 0 {
		suggestedSpeed = 0
	}
	return suggestedSpeed
}

func (s *State) UpdateCarState(carData car.CarState) {
	s.LastCarSetSpeed = s.CarSetSpeed
	carSetSpeed := carData.VCruise() * ms.KPH_TO_MS
	if s.CarSetSpeed != carSetSpeed {
		s.CarSetSpeed = carSetSpeed
		s.TimeLastSetSpeedAdjust = time.Now()
	}
	s.CarVEgo = carData.VEgo()
	s.CarAEgo = carData.AEgo()
	s.CarVCruise = carData.VCruise()
	s.GasPressed = carData.GasPressed()
	tdiff := time.Since(s.TimeLastCarState)
	seconds := s.CarStateUpdateTimeMA.Update(tdiff.Seconds())
	s.DistanceSinceLastPosition += float32(seconds) * s.CarVEgo
	if ms.Settings.PressGasToOverrideSpeedLimit && s.GasPressed {
		s.SpeedLimitOverrideSpeed = s.CarVEgo
	}
}

func (s *State) Send() error {
	msg, output := s.Publisher.NewMessage(true)

	name := s.CurrentWay.Way.WayName()
	output.SetWayName(name)

	ref := s.CurrentWay.Way.WayRef()
	output.SetWayRef(ref)

	output.SetRoadName(s.CurrentWay.Way.Name())

	maxSpeed := s.CurrentWay.Way.MaxSpeed()
	output.SetSpeedLimit(float32(maxSpeed))

	speedLimitSuggestion := s.SpeedLimit()
	output.SetSpeedLimitSuggestedSpeed(speedLimitSuggestion)

	output.SetNextSpeedLimit(s.NextSpeedLimit.Value)
	output.SetNextSpeedLimitDistance(s.NextSpeedLimit.Distance)

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

func (s *State) SpeedLimit() float32 {
	slSuggestedSpeed := ms.Settings.PrioritySpeedLimit(float32(s.MaxSpeed))
	if slSuggestedSpeed == 0 && ms.Settings.HoldLastSeenSpeedLimit {
		slSuggestedSpeed = float32(s.LastSpeedLimitValue)
	}
	if slSuggestedSpeed > 0 {
		slSuggestedSpeed += ms.Settings.SpeedLimitOffset
	}
	if s.NextSpeedLimit.Value > 0 {
		offsetNextSpeedLimit := s.NextSpeedLimit.Value + ms.Settings.SpeedLimitOffset
		speedLimit := ms.Settings.PrioritySpeedLimit(float32(s.MaxSpeed))
		nextIsLower := speedLimit > s.NextSpeedLimit.Value
		distanceToReachSpeed := CalculateJerkLimitedDistanceSimple(s.CarVEgo, s.CarAEgo, offsetNextSpeedLimit, ms.Settings.TargetSpeedAccel, ms.Settings.TargetSpeedJerk)
		distanceToReachSpeed += ms.Settings.TargetSpeedTimeOffset * s.CarVEgo
		if s.NextSpeedLimit.TriggerDistance > distanceToReachSpeed {
			distanceToReachSpeed = s.NextSpeedLimit.TriggerDistance
		}
		if distanceToReachSpeed > s.NextSpeedLimit.TriggerDistance {
			s.NextSpeedLimit.Distance = distanceToReachSpeed + 10
		}
		if !nextIsLower && ms.Settings.SpeedUpForNextSpeedLimit && s.NextSpeedLimit.Distance < distanceToReachSpeed {
			slSuggestedSpeed = float32(offsetNextSpeedLimit)
		} else if nextIsLower && ms.Settings.SlowDownForNextSpeedLimit && s.NextSpeedLimit.Distance < distanceToReachSpeed {
			slSuggestedSpeed = float32(offsetNextSpeedLimit)
		}
	}
	return slSuggestedSpeed
}

func abs[T float64 | float32](val T) float64 {
	return math.Abs(float64(val))
}
