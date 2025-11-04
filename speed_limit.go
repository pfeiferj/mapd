package main

import (
	"strconv"
	"strings"

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

type NextSpeedLimit struct {
	Latitude   float64
	Longitude  float64
	Speedlimit float64
	Distance   float64
}
