package utils

import (
	"time"

	m "pfeifer.dev/mapd/math"
)

type UpdateTracker struct {
	LastTime time.Time
	Time     time.Time
	DiffMA   m.MovingAverage
	skipNext bool
}

func (u *UpdateTracker) Init(maLength int) {
	u.LastTime = time.Now()
	u.Time = time.Now()
	u.DiffMA.Init(maLength)
}

func (u *UpdateTracker) Rebase() {
	now := time.Now()
	u.LastTime = now
	u.Time = now
	u.skipNext = true
}

func (u *UpdateTracker) Update() {
	u.LastTime = u.Time
	u.Time = time.Now()
	if u.skipNext {
		u.skipNext = false
		return
	}
	u.DiffMA.Update(u.Time.Sub(u.LastTime).Seconds())
}
