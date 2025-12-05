package utils

import (
	"math"
	"time"
)

type TrackedState[T any] struct {
	LastValue          T
	Value              T
	UpdatedTime        time.Time
	Equal              func(T, T) bool
	Null               func(T) bool
	AllowNullLastValue bool
}

func (t *TrackedState[T]) Update(val T) (updated bool) {
	if !t.Equal(t.Value, val) {
		if t.AllowNullLastValue || !t.Null(t.LastValue) {
			t.LastValue = t.Value
		}
		t.UpdatedTime = time.Now()
		t.Value = val
		return true
	}
	return false
}

func Float32Compare(a float32, b float32) bool {
	return a == b
}

func Float32Null(a float32) bool {
	return math.IsNaN(float64(a)) || a == 0
}
