package main

import (
	"log/slog"
	"math"
	"fmt"

	ms "pfeifer.dev/mapd/settings"
)

//func calculate_accel(t float32, target_jerk float32, a_ego float32) float32 {
//  return a_ego  + target_jerk * t
//}

func calculate_velocity(t float32, target_jerk float32, a_ego float32, v_ego float32) float32 {
  return v_ego + a_ego * t + target_jerk/2 * (t * t)
}

func calculate_distance(t float32, target_jerk float32, a_ego float32, v_ego float32) float32 {
  return t * v_ego + a_ego/2 * (t * t) + target_jerk/6 * (t * t * t)
}

func UpdateCurveSpeed(s *State) {
	var distances = make([]float64, len(s.TargetVelocities))
	min_dist := float64(10000)
	min_idx := 0
	for i, tv := range s.TargetVelocities {
		d := DistanceToPoint(
			s.Location.Latitude() * ms.TO_RADIANS,
			s.Location.Longitude() * ms.TO_RADIANS,
			tv.Latitude * ms.TO_RADIANS,
			tv.Longitude * ms.TO_RADIANS,
			)

		distances[i] = d
		if d < min_dist {
			min_dist = d
			min_idx = i
		}
	}
	forwardSize := len(s.TargetVelocities) - min_idx
	var forwardPoints = make([]Velocity, forwardSize)
	var forwardDistances = make([]float64, forwardSize)
	

	for i := range forwardSize {
		forwardPoints[i] = s.TargetVelocities[i + min_idx]
		forwardDistances[i] = distances[i + min_idx]
	}

	minValidV := float32(1000)
	fmt.Printf("%v", forwardDistances)
	for i, d := range forwardDistances {
		tv := forwardPoints[i]
		if tv.Velocity > float64(s.CarVEgo) {
			continue
		}
		a_diff := s.CarAEgo - ms.Settings.CurveTargetAccel
		accel_t := math.Abs(float64(a_diff / ms.Settings.CurveTargetJerk))
		min_accel_v := calculate_velocity(float32(accel_t), ms.Settings.CurveTargetJerk, s.CarAEgo, s.CarVEgo)
		max_d := float32(0)
		if float32(tv.Velocity) > min_accel_v {
			// calculate time needed based on target jerk
			a := float32(0.5 * ms.Settings.CurveTargetJerk)
			b := s.CarAEgo
			c := s.CarVEgo - float32(tv.Velocity)
			t_a := -1 * (float32(math.Sqrt(float64(b*b - 4 * a * c))) + b) / 2 * a
			t := (float32(math.Sqrt(float64(b*b - 4 * a * c))) - b) / 2 * a
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

		slog.Debug("", "a", d, "b", max_d + float32(tv.Velocity) * ms.Settings.CurveTargetOffset)
		if float32(d) < max_d + float32(tv.Velocity) * ms.Settings.CurveTargetOffset {
			slog.Debug("valid v!", "v", tv.Velocity)
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
	slog.Debug("done calculating curve speed", "curve speed", s.CurveSpeed)
}

