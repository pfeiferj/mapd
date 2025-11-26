package main

import (
	ms "pfeifer.dev/mapd/settings"
	m "pfeifer.dev/mapd/math"
)

//func calculate_accel(t float32, target_jerk float32, a_ego float32) float32 {
//  return a_ego  + target_jerk * t
//}

func calculate_velocity(t float32, target_jerk float32, a_ego float32, v_ego float32) float32 {
	return v_ego + a_ego*t + target_jerk/2*(t*t)
}

func calculate_distance(t float32, target_jerk float32, a_ego float32, v_ego float32) float32 {
	return t*v_ego + a_ego/2*(t*t) + target_jerk/6*(t*t*t)
}

func UpdateCurveSpeed(s *State) {
	distances := make([]float32, len(s.TargetVelocities))
	match_idx := -1
	for i, tv := range s.TargetVelocities {
		d := s.CurrentWay.OnWay.Distance.LinePosition.Pos.DistanceTo(tv.Pos)

		distances[i] = d

		// find index of the most recent node we have driven past based on which node was used to calculate if we are on the way
		if tv.Pos.Equals(s.CurrentWay.Distance.LineStart) && match_idx == -1 {
			match_idx = i + 1
		}
		if tv.Pos.Equals(s.CurrentWay.Distance.LineEnd) && match_idx == -1 {
			match_idx = i + 1
		}
	}
	if match_idx == -1 {
		match_idx = 0
	}
	forwardSize := len(s.TargetVelocities) - match_idx
	forwardPoints := make([]Velocity, forwardSize)
	forwardDistances := make([]float32, forwardSize)

	pastTriggerPos := true
	for i := range forwardSize {
		forwardPoints[i] = s.TargetVelocities[i+match_idx]
		if forwardPoints[i].Pos.Equals(s.MapCurveTriggerPos) {
			pastTriggerPos = false
		}
		forwardDistances[i] = distances[i+match_idx] - s.DistanceSinceLastPosition
		if forwardDistances[i] <= 0 {
			forwardDistances[i] = distances[i+match_idx]
		}
	}

	minValidV := float32(1000)
	minValidPos := m.Position{}
	if pastTriggerPos {
		s.MapCurveTriggerSpeed = max(s.MapCurveTriggerSpeed, s.CarVEgo + ms.CURVE_CALC_OFFSET)
	}
	calcSpeed := s.CarVEgo
	if s.MapCurveTriggerSpeed > 0 && s.MapCurveTriggerSpeed > s.CarVEgo {
		calcSpeed = s.MapCurveTriggerSpeed
	}
	for i, d := range forwardDistances {
		tv := forwardPoints[i]
		if tv.Velocity > float64(calcSpeed) {
			continue
		}

		max_d := s.DistanceToReachSpeed(tv.Velocity, calcSpeed)

		if float32(d) < max_d+float32(tv.Velocity)*ms.Settings.CurveTargetOffset {
			if float32(tv.Velocity) < minValidV {
				minValidV = float32(tv.Velocity)
				minValidPos = tv.Pos
			}
		}
	}
	if minValidV == float32(1000) {
		s.CurveSpeed = 0
		if s.MapCurveTriggerSpeed > s.SuggestedSpeed() {
			s.MapCurveTriggerSpeed = 0
		}
	} else {
		s.CurveSpeed = minValidV
		if s.MapCurveTriggerSpeed - s.CarVEgo > ms.CURVE_CALC_OFFSET {
			s.MapCurveTriggerSpeed = s.CarVEgo + ms.CURVE_CALC_OFFSET
		}
		if s.MapCurveTriggerSpeed == 0 {
			s.MapCurveTriggerSpeed = s.CarVEgo + ms.CURVE_CALC_OFFSET
			s.MapCurveTriggerPos = minValidPos 
		}
	}
}
