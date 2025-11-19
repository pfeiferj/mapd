package main

import (
	"math"

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
	calcSpeed := s.CarVEgo
	if s.MapCurveTriggerSpeed > 0 && s.MapCurveTriggerSpeed > s.CarVEgo {
		calcSpeed = s.MapCurveTriggerSpeed
	} else {
		s.MapCurveTriggerSpeed = 0
	}
	for i, d := range forwardDistances {
		tv := forwardPoints[i]
		if tv.Velocity > float64(calcSpeed) {
			continue
		}
		a_diff := s.CarAEgo - ms.Settings.CurveTargetAccel
		accel_t := math.Abs(float64(a_diff / ms.Settings.CurveTargetJerk))
		min_accel_v := calculate_velocity(float32(accel_t), ms.Settings.CurveTargetJerk, s.CarAEgo, calcSpeed)
		max_d := float32(0)
		if float32(tv.Velocity) > min_accel_v {
			// calculate time needed based on target jerk
			a := float32(0.5 * ms.Settings.CurveTargetJerk)
			b := s.CarAEgo
			c := calcSpeed - float32(tv.Velocity)
			t_a := -1 * (float32(math.Sqrt(float64(b*b-4*a*c))) + b) / 2 * a
			t := (float32(math.Sqrt(float64(b*b-4*a*c))) - b) / 2 * a
			if !math.IsNaN(float64(t_a)) && t_a > 0 {
				t = t_a
			}
			if math.IsNaN(float64(t)) {
				continue
			}

			max_d = calculate_distance(t, ms.Settings.CurveTargetJerk, s.CarAEgo, s.CarVEgo)
		} else {
			max_d = calculate_distance(float32(accel_t), ms.Settings.CurveTargetJerk, s.CarAEgo, s.CarVEgo)
			// calculate additional time needed based on target accel
			t := math.Abs(float64((min_accel_v - float32(tv.Velocity)) / ms.Settings.CurveTargetAccel))
			max_d += calculate_distance(float32(t), 0, ms.Settings.CurveTargetAccel, min_accel_v)

		}

		if float32(d) < max_d+float32(tv.Velocity)*ms.Settings.CurveTargetOffset {
			if float32(tv.Velocity) < minValidV {
				minValidV = float32(tv.Velocity)
			}
		}
	}
	if minValidV == float32(1000) {
		s.CurveSpeed = 0
		s.MapCurveTriggerSpeed = 0
	} else {
		s.CurveSpeed = minValidV
		if s.MapCurveTriggerSpeed == 0 {
			s.MapCurveTriggerSpeed = s.CarVEgo
		}
	}
}
