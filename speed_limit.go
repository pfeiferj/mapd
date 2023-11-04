package main

import (
	"strconv"
	"strings"
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
		return 0.277778 * float64(numeric)
	}

	if splitSpeed[1] == "kph" || splitSpeed[1] == "km/h" || splitSpeed[1] == "kmh" {
		return 0.277778 * float64(numeric)
	} else if splitSpeed[1] == "mph" {
		return 0.44704 * float64(numeric)
	} else if splitSpeed[1] == "knots" {
		return 0.514444 * float64(numeric)
	}

	return 0
}
