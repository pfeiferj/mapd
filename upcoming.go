package main

import (
	"math"

	"pfeifer.dev/mapd/maps"
	m "pfeifer.dev/mapd/math"
)

func NewUpcoming[T any](maLength int, defaultValue T, checkWay CheckWay[T]) Upcoming[T] {
	u := Upcoming[T]{
		CheckWay:     checkWay,
		DefaultValue: defaultValue,
		Value:        defaultValue,
	}
	u.DistanceMA.Init(maLength)
	return u
}

type CheckWay[T any] func(*State, *Upcoming[T], maps.NextWayResult) (valid bool, val T)

type Upcoming[T any] struct {
	CheckWay        CheckWay[T]
	DefaultValue    T
	Value           T
	Position        m.Position
	Distance        float32
	RawDistance     float32
	DistanceMA      m.MovingAverage
	TriggerDistance float32
}

func (u *Upcoming[T]) Reset() {
	u.DistanceMA.Reset()
	u.Distance = 0
	u.RawDistance = 0
	u.TriggerDistance = 0
	u.Value = u.DefaultValue
	u.Position = m.Position{}
}

func (u *Upcoming[T]) Update(state *State) {
	if len(state.NextWays) == 0 {
		u.Reset()
		return
	}

	cumulativeDistance := float32(0.0)

	if len(state.CurrentWay.Way.Nodes()) > 0 {
		distToEnd, err := state.CurrentWay.Way.DistanceToEnd(state.Position, state.CurrentWay.OnWay.IsForward)
		if err == nil && distToEnd > 0 {
			cumulativeDistance = distToEnd - state.DistanceSinceLastPosition
			cumulativeDistance = max(cumulativeDistance, 0)
		}
	}
	for _, nextWay := range state.NextWays {
		valid, val := u.CheckWay(state, u, nextWay)
		if valid {
			if u.Position.Equals(nextWay.StartPosition) {
				diff := float64(u.Distance - cumulativeDistance)
				if math.Abs(float64(diff)) > 100 { // something bad happened, reset state
					u.Distance = cumulativeDistance
					state.DistanceSinceLastPosition = 0
					diff = 0
					u.DistanceMA.Reset()
				}
				smoothed_diff := diff

				// only update on position updates and resets
				if state.DistanceSinceLastPosition == 0 {
					smoothed_diff = u.DistanceMA.Update(diff)
				}

				u.Distance = u.Distance - float32(smoothed_diff)
			} else {
				u.DistanceMA.Reset()
				u.Distance = cumulativeDistance
				u.TriggerDistance = 0
			}
			u.Position = nextWay.StartPosition
			u.RawDistance = cumulativeDistance
			u.Value = val
			return
		}
		cumulativeDistance += nextWay.Way.Distance()
	}

	// No upcoming way found, reset state
	u.Reset()
}
