package main

import (
	ms "pfeifer.dev/mapd/settings"
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

	for i := range forwardSize {
		forwardPoints[i] = s.TargetVelocities[i+match_idx]
		forwardDistances[i] = distances[i+match_idx] - s.DistanceSinceLastPosition
		if forwardDistances[i] <= 0 {
			forwardDistances[i] = distances[i+match_idx]
		}
	}

	minValidV := float32(1000)
	for i, d := range forwardDistances {
		tv := forwardPoints[i]
		if tv.Velocity > float64(s.CarVEgo) + ms.CURVE_CALC_OFFSET {
			continue
		}

		calcSpeed := s.CarVEgo
		if tv.CalcSpeed > 0 && tv.CalcSpeed > calcSpeed {
			calcSpeed = tv.CalcSpeed
		}

		max_d := s.DistanceToReachSpeed(tv.Velocity, calcSpeed)

		if float32(d) < max_d || (tv.Velocity > float64(s.CarVEgo) && tv.CalcSpeed > 0) {
			if s.CarVEgo + ms.CURVE_CALC_OFFSET > tv.CalcSpeed {
				tv.CalcSpeed = s.CarVEgo + ms.CURVE_CALC_OFFSET
			}
			if float32(tv.Velocity) < minValidV {
				minValidV = float32(tv.Velocity)
			}
		}
	}
	if minValidV == float32(1000) {
		s.CurveSpeed = 0
	} else {
		s.CurveSpeed = minValidV
	}
}
