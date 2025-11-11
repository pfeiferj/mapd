package main

import (
	"log/slog"
	"math"
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

func calculateNextSpeedLimit(state *State, currentMaxSpeed float64) NextSpeedLimit {
	if len(state.NextWays) == 0 {
		return NextSpeedLimit{}
	}

	// Find the next speed limit change
	cumulativeDistance := 0.0

	if state.CurrentWay.Way.HasNodes() {
		distToEnd, err := DistanceToEndOfWay(state.Location.Latitude(), state.Location.Longitude(), state.CurrentWay.Way, state.CurrentWay.OnWay.IsForward)
		if err == nil && distToEnd > 0 {
			cumulativeDistance = distToEnd - state.CurrentWay.OnWay.Distance.Distance - float64(state.DistanceSinceLastPosition)
		}
	}

	// Look through next ways for speed limit change
	for _, nextWay := range state.NextWays {
		nextMaxSpeed := nextWay.Way.MaxSpeed()
		if nextWay.IsForward && nextWay.Way.MaxSpeedForward() > 0 {
			nextMaxSpeed = nextWay.Way.MaxSpeedForward()
		} else if !nextWay.IsForward && nextWay.Way.MaxSpeedBackward() > 0 {
			nextMaxSpeed = nextWay.Way.MaxSpeedBackward()
		}

		if nextMaxSpeed != currentMaxSpeed && nextMaxSpeed > 0 {
			result := NextSpeedLimit{
				Latitude:   nextWay.StartPosition.Latitude(),
				Longitude:  nextWay.StartPosition.Longitude(),
				Speedlimit: nextMaxSpeed,
				Distance:   cumulativeDistance,
			}

			wayName := RoadName(nextWay.Way)
			if nextMaxSpeed == state.LastSpeedLimitValue && wayName == state.LastSpeedLimitWayName {
				diff := state.LastSpeedLimitDistance - cumulativeDistance
				if math.Abs(diff) > 100 { // something bad happened, reset state
					state.LastSpeedLimitDistance = cumulativeDistance
					state.DistanceSinceLastPosition = 0
					diff = 0
					state.NextSpeedLimitMA.Reset()
				}
				smoothed_diff := diff
				if state.DistanceSinceLastPosition == 0 {
					smoothed_diff = state.NextSpeedLimitMA.Update(diff)
				}
				result.Distance = state.LastSpeedLimitDistance - smoothed_diff

				slog.Debug("Smoothed speed limit distance",
					"raw_distance", cumulativeDistance,
					"smoothed_distance", result.Distance,
					"last_distance", state.LastSpeedLimitDistance,
					"way", wayName,
				)

			} else {
				state.NextSpeedLimitMA.Reset()
			}
			state.LastSpeedLimitDistance = result.Distance
			state.LastSpeedLimitValue = nextMaxSpeed
			state.LastSpeedLimitWayName = wayName

			return result
		}
		if nextWay.Way.HasNodes() {
			wayDistance, err := calculateWayDistance(nextWay.Way)
			if err == nil {
				cumulativeDistance += wayDistance
			}
		}
	}

	return NextSpeedLimit{}
}
