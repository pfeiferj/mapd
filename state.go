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
	return math.Abs(float64(s.CarSetSpeed-ms.Settings.EnableSpeed)) < ms.ENABLE_SPEED_RANGE
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

func (s *State) DistanceToReachSpeed(targetV float64, calcSpeed float32) float32 {
	// Always use absolute values and apply correct sign based on direction
	targetA := float32(math.Abs(float64(ms.Settings.CurveTargetAccel)))
	targetJ := float32(math.Abs(float64(ms.Settings.CurveTargetJerk)))

	// Apply negative sign for deceleration
	if targetV <= float64(calcSpeed) {
		targetA = -targetA
		targetJ = -targetJ
	}

	a_diff := s.CarAEgo - targetA
	if targetV > float64(calcSpeed) {
		a_diff = targetA - s.CarAEgo
	}
	accel_t := float64(a_diff / targetJ)
	min_accel_v := calculate_velocity(float32(accel_t), targetJ, s.CarAEgo, calcSpeed)
	max_d := float32(0)
	if float32(targetV) > min_accel_v {
		// calculate time needed based on target jerk
		// solving: targetV = calcSpeed + s.CarAEgo*t + (targetJ/2)*t²
		a := float32(0.5 * targetJ)
		b := s.CarAEgo
		c := float32(float64(calcSpeed) - targetV)

		// Check discriminant before computing roots
		discriminant := b*b - 4*a*c
		if discriminant < 0 {
			return 0 // No real solution exists
		}

		sqrtDiscriminant := float32(math.Sqrt(float64(discriminant)))

		// Calculate both roots
		t1 := (-b + sqrtDiscriminant) / (2 * a)
		t2 := (-b - sqrtDiscriminant) / (2 * a)

		// Select the smallest positive root
		var t float32
		if t1 > 0 && t2 > 0 {
			t = min(t1, t2)
		} else if t1 > 0 {
			t = t1
		} else if t2 > 0 {
			t = t2
		} else {
			return 0 // No positive solution (would mean negative time)
		}

		max_d = calculate_distance(t, targetJ, s.CarAEgo, s.CarVEgo)
	} else {
		max_d = calculate_distance(float32(accel_t), targetJ, s.CarAEgo, s.CarVEgo)
		// calculate additional time needed based on target accel
		t := math.Abs(float64((min_accel_v - float32(targetV)) / targetA))
		max_d += calculate_distance(float32(t), 0, targetA, min_accel_v)
	}
	return max_d + float32(targetV) * ms.Settings.CurveTargetOffset
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
		calcSpeed := s.CarVEgo
		speedLimit := ms.Settings.PrioritySpeedLimit(float32(s.MaxSpeed))
		nextIsLower := speedLimit > float32(s.NextSpeedLimit.Speedlimit)
		if s.NextSpeedLimit.CalcSpeed > s.CarVEgo && nextIsLower {
			calcSpeed = s.NextSpeedLimit.CalcSpeed
		} else if s.NextSpeedLimit.CalcSpeed < s.CarVEgo && !nextIsLower {
			calcSpeed = s.NextSpeedLimit.CalcSpeed
		}
		distanceToReachSpeed := s.DistanceToReachSpeed(s.NextSpeedLimit.Speedlimit, calcSpeed)
		if !nextIsLower && ms.Settings.SpeedUpForNextSpeedLimit && s.NextSpeedLimit.Distance < distanceToReachSpeed {
			if s.CarVEgo - ms.CURVE_CALC_OFFSET < s.NextSpeedLimit.CalcSpeed {
				s.NextSpeedLimit.CalcSpeed = s.CarVEgo - ms.CURVE_CALC_OFFSET
			}
			slSuggestedSpeed = float32(offsetNextSpeedLimit)
		} else if nextIsLower && ms.Settings.SlowDownForNextSpeedLimit && s.NextSpeedLimit.Distance < distanceToReachSpeed {
			if s.CarVEgo + ms.CURVE_CALC_OFFSET > s.NextSpeedLimit.CalcSpeed || s.NextSpeedLimit.CalcSpeed == 0 {
				s.NextSpeedLimit.CalcSpeed = s.CarVEgo + ms.CURVE_CALC_OFFSET
			}
			slSuggestedSpeed = float32(offsetNextSpeedLimit)
		}
	}
	return slSuggestedSpeed
}

