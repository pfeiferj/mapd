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
	NextSpeedLimit            NextSpeedLimit
	VisionCurveSpeed          float32
	CarSetSpeed               float32
	TimeLastSetSpeedAdjust    time.Time
	CarVEgo                   float32
	CarAEgo                   float32
	CarVCruise                float32
	CurveSpeed                float32
	NextSpeedLimitMA          m.MovingAverage
	VisionCurveMA             m.MovingAverage
	CarStateUpdateTimeMA      m.MovingAverage
	DistanceSinceLastPosition float32
	TimeLastPosition          time.Time
	TimeLastModel             time.Time
	TimeLastCarState          time.Time
}

func (s *State) checkEnableSpeed() bool {
	if ms.Settings.EnableSpeed == 0 {
		return true
	}
	return abs(s.CarSetSpeed-ms.Settings.EnableSpeed) < ms.ENABLE_SPEED_RANGE
}

func (s *State) SuggestedSpeed() float32 {
	suggestedSpeed := min(s.CarVCruise * ms.KPH_TO_MS, ms.MAX_OP_SPEED)
	setSpeedChanging := time.Since(s.TimeLastSetSpeedAdjust) < 1500*time.Millisecond

	if ms.Settings.SpeedLimitControlEnabled || ms.Settings.ExternalSpeedLimitControlEnabled {
		slSuggestedSpeed := s.SpeedLimit()
		if slSuggestedSpeed != s.LastSpeedLimitSuggestion {
			ms.Settings.ResetSpeedLimitAccepted()
		}
		if ms.Settings.SpeedLimitAccepted() {
			s.AcceptedSpeedLimitValue = slSuggestedSpeed
		}
		if suggestedSpeed > slSuggestedSpeed {
			if !ms.Settings.SpeedLimitUseEnableSpeed || s.checkEnableSpeed() {
				suggestedSpeed = s.AcceptedSpeedLimitValue
			} else if setSpeedChanging && ms.Settings.HoldSpeedLimitWhileChangingSetSpeed && s.CarVEgo-1 < slSuggestedSpeed {
				suggestedSpeed = s.AcceptedSpeedLimitValue
			}
		}
		s.LastSpeedLimitSuggestion = slSuggestedSpeed
	}
	if ms.Settings.VisionCurveSpeedControlEnabled && s.VisionCurveSpeed > 0 && (s.VisionCurveSpeed < suggestedSpeed || suggestedSpeed == 0) && (!ms.Settings.VisionCurveUseEnableSpeed || s.checkEnableSpeed()) {
		suggestedSpeed = s.VisionCurveSpeed
	}
	if ms.Settings.CurveSpeedControlEnabled && s.CurveSpeed > 0 && (s.CurveSpeed < suggestedSpeed || suggestedSpeed == 0) && (!ms.Settings.CurveUseEnableSpeed || s.checkEnableSpeed()) {
		suggestedSpeed = s.CurveSpeed
	}
	if suggestedSpeed < 0 {
		suggestedSpeed = 0
	}
	return suggestedSpeed
}

func (s *State) UpdateCarState(carData car.CarState) {
	carSetSpeed := carData.VCruise() * ms.KPH_TO_MS
	if s.CarSetSpeed != carSetSpeed {
		s.CarSetSpeed = carSetSpeed
		s.TimeLastSetSpeedAdjust = time.Now()
	}
	s.CarVEgo = carData.VEgo()
	s.CarAEgo = carData.AEgo()
	s.CarVCruise = carData.VCruise()
	tdiff := time.Since(s.TimeLastCarState)
	seconds := s.CarStateUpdateTimeMA.Update(tdiff.Seconds())
	s.DistanceSinceLastPosition += float32(seconds) * s.CarVEgo
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

	output.SetNextSpeedLimit(float32(s.NextSpeedLimit.Speedlimit))
	output.SetNextSpeedLimitDistance(float32(s.NextSpeedLimit.Distance))

	hazard := s.CurrentWay.Way.Hazard()
	output.SetHazard(hazard)

	advisorySpeed := s.CurrentWay.Way.AdvisorySpeed()
	output.SetAdvisorySpeed(float32(advisorySpeed))

	oneWay := s.CurrentWay.Way.OneWay()
	output.SetOneWay(oneWay)

	lanes := s.CurrentWay.Way.Lanes()
	output.SetLanes(uint8(lanes))

	output.SetTileLoaded(s.Data.Loaded)

	output.SetRoadContext(custom.RoadContext(s.CurrentWay.Way.Context()))
	output.SetEstimatedRoadWidth(s.CurrentWay.Way.Width())
	output.SetVisionCurveSpeed(s.VisionCurveSpeed)
	output.SetCurveSpeed(s.CurveSpeed)

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
	if s.NextSpeedLimit.Speedlimit > 0 {
		offsetNextSpeedLimit := s.NextSpeedLimit.Speedlimit + float64(ms.Settings.SpeedLimitOffset)
		speedLimit := ms.Settings.PrioritySpeedLimit(float32(s.MaxSpeed))
		nextIsLower := speedLimit > float32(s.NextSpeedLimit.Speedlimit)
		distanceToReachSpeed := CalculateJerkLimitedDistanceSimple(s.CarVEgo, s.CarAEgo, float32(offsetNextSpeedLimit), ms.Settings.CurveTargetAccel, ms.Settings.CurveTargetJerk)
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
