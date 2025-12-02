package main

import (
	"strconv"
	"strings"

	"pfeifer.dev/mapd/maps"
	ms "pfeifer.dev/mapd/settings"
)

func ParseMaxSpeed(maxspeed string) float64 {
	splitSpeed := strings.Split(maxspeed, " ")
	if len(splitSpeed) == 0 {
		return 0
	}

	numeric, err := strconv.ParseUint(splitSpeed[0], 10, 64)
	if err != nil {
		return 0
	}

	if len(splitSpeed) == 1 {
		return ms.KPH_TO_MS * float64(numeric)
	}

	if splitSpeed[1] == "kph" || splitSpeed[1] == "km/h" || splitSpeed[1] == "kmh" {
		return ms.KPH_TO_MS * float64(numeric)
	} else if splitSpeed[1] == "mph" {
		return ms.MPH_TO_MS * float64(numeric)
	} else if splitSpeed[1] == "knots" {
		return ms.KNOTS_TO_MS * float64(numeric)
	}

	return 0
}

func checkWayForSpeedLimitChange(state *State, parent *Upcoming[float32], way maps.NextWayResult) (valid bool, val float32) {
	nextMaxSpeed := way.Way.MaxSpeed()
	if way.IsForward && way.Way.MaxSpeedForward() > 0 {
		nextMaxSpeed = way.Way.MaxSpeedForward()
	} else if !way.IsForward && way.Way.MaxSpeedBackward() > 0 {
		nextMaxSpeed = way.Way.MaxSpeedBackward()
	}

	if nextMaxSpeed != state.MaxSpeed && nextMaxSpeed > 0 {
		return true, float32(nextMaxSpeed)
	}

	return false, parent.DefaultValue
}

func checkWayForAdvisorySpeedChange(state *State, parent *Upcoming[float32], way maps.NextWayResult) (valid bool, val float32) {

	nextAdvisorySpeed := way.Way.AdvisorySpeed()

	if nextAdvisorySpeed != state.CurrentWay.Way.AdvisorySpeed() && nextAdvisorySpeed > 0 {
		return true, float32(nextAdvisorySpeed)
	}

	return false, parent.DefaultValue
}
