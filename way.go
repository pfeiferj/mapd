package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/cereal/offline"
	"pfeifer.dev/mapd/maps"
	ms "pfeifer.dev/mapd/settings"
	m "pfeifer.dev/mapd/math"
)

type WayCandidate struct {
	Way              offline.Way
	OnWayResult      maps.OnWayResult
	BearingAlignment float32 // sin(bearing_delta) - lower is better
	DistanceToWay    float32
	HierarchyRank    int
	Context          maps.RoadContext
}


// Updated CurrentWay struct with stability fields
type CurrentWay struct {
	Way               maps.Way
	Distance          maps.DistanceResult
	OnWay             maps.OnWayResult
	StartPosition     m.Position
	EndPosition       m.Position
	ConfidenceCounter int
	LastChangeTime    time.Time
	StableDistance    float32
	SelectionType     custom.WaySelectionType
}

func selectBestWayAdvanced(possibleWays []maps.Way, location log.GpsLocationData, currentWay maps.Way) maps.Way {
	if len(possibleWays) == 0 {
		return maps.Way{}
	}
	if len(possibleWays) == 1 {
		return possibleWays[0]
	}

	bestWay := possibleWays[0]
	bestScore := float32(-1000)

	for _, way := range possibleWays {
		onWay, err := way.OnWay(location, 1)
		if err != nil || !onWay.OnWay {
			continue
		}

		score := float32(0)

		score += float32(100 - way.Rank())

		bearingAlignment, err := way.BearingAlignment(location)
		if err == nil {
			score += (1.0 - bearingAlignment) * 50
		}

		score -= onWay.Distance.Distance * 0.1

		if len(currentWay.Nodes()) > 0 {
			currentName := currentWay.WayName()
			currentRef := currentWay.WayRef()
			wayName := way.WayName()
			wayRef := way.WayRef()

			if len(currentName) > 0 && currentName == wayName {
				score += 30.0
			}
			if len(currentRef) > 0 && currentRef == wayRef {
				score += 25.0
			}
		}

		if score > bestScore {
			bestScore = score
			bestWay = way
		}
	}

	return bestWay
}

func GetCurrentWay(currentWay CurrentWay, nextWays []maps.NextWayResult, offline *maps.Offline, location log.GpsLocationData) (CurrentWay, error) {
	distanceFromCurrentWay := currentWay.OnWay.Distance.Distance
	nodes := currentWay.Way.Nodes()
	if len(nodes) > 1 {
		onWay, err := currentWay.Way.OnWay(location, currentWay.Way.DistanceMultiplier())
		newStableDistance := onWay.Distance.Distance
		distanceFromCurrentWay = newStableDistance
		t := onWay.Distance.LinePosition.T
		isEdge := t == 1 || t == 0
		if err == nil && onWay.OnWay && !isEdge {
			start, end := currentWay.Way.GetStartEnd(onWay.IsForward)
			return CurrentWay{
				Way:               currentWay.Way,
				Distance:          onWay.Distance,
				OnWay:             onWay,
				StartPosition:     start,
				EndPosition:       end,
				ConfidenceCounter: currentWay.ConfidenceCounter + 1,
				LastChangeTime:    currentWay.LastChangeTime,
				StableDistance:    newStableDistance,
				SelectionType: custom.WaySelectionType_current,
			}, nil
		}
		if err != nil {
			slog.Debug("failed to check if on way", "error", err)
		}
	}

	for _, nextWay := range nextWays {
		if len(nextWay.Way.Nodes()) == 0 {
			continue
		}
		onWay, err := nextWay.Way.OnWay(location, nextWay.Way.DistanceMultiplier())
		if err == nil && onWay.OnWay {
			start, end := nextWay.Way.GetStartEnd(onWay.IsForward)
			return CurrentWay{
				Way:               nextWay.Way,
				Distance:          onWay.Distance,
				OnWay:             onWay,
				StartPosition:     start,
				EndPosition:       end,
				ConfidenceCounter: 1,
				LastChangeTime:    time.Now(),
				StableDistance:    onWay.Distance.Distance,
				SelectionType: custom.WaySelectionType_predicted,
			}, nil
		}
		if err != nil {
			slog.Debug("failed to check if on way", "error", err)
		}
	}

	possibleWays, err := getPossibleWays(offline, location)
	if err == nil && len(possibleWays) > 0 {
		selectedWay := selectBestWayAdvanced(possibleWays, location, currentWay.Way)
		if len(selectedWay.Nodes()) > 0 {
			selectedOnWay, err := selectedWay.OnWay(location, selectedWay.DistanceMultiplier())
			if err == nil && selectedOnWay.OnWay {
				start, end := selectedWay.GetStartEnd(selectedOnWay.IsForward)
				return CurrentWay{
					Way:               selectedWay,
					Distance:          selectedOnWay.Distance,
					OnWay:             selectedOnWay,
					StartPosition:     start,
					EndPosition:       end,
					ConfidenceCounter: 1,
					LastChangeTime:    time.Now(),
					StableDistance:    selectedOnWay.Distance.Distance,
					SelectionType: custom.WaySelectionType_possible,
				}, nil
			}
			if err != nil {
				slog.Debug("failed to check if on way", "error", err)
			}
		}
	}

	if len(currentWay.Way.Nodes()) > 0 {
		onWay, err := currentWay.Way.OnWay(location, 2)
		if err == nil && onWay.OnWay {
			start, end := currentWay.Way.GetStartEnd(onWay.IsForward)
			return CurrentWay{
				Way:               currentWay.Way,
				Distance:          onWay.Distance,
				OnWay:             onWay,
				StartPosition:     start,
				EndPosition:       end,
				ConfidenceCounter: currentWay.ConfidenceCounter,
				LastChangeTime:    currentWay.LastChangeTime,
				StableDistance:    currentWay.StableDistance,
				SelectionType:     custom.WaySelectionType_extended,
			}, nil
		}
		if err != nil {
			slog.Debug("failed to check if on way", "error", err)
		}
	}

	return CurrentWay{SelectionType: custom.WaySelectionType_fail}, errors.New(fmt.Sprintf("could not find a current way, distance from last way=%f", distanceFromCurrentWay))
}

func getPossibleWays(offlineMaps *maps.Offline, location log.GpsLocationData) ([]maps.Way, error) {
	possibleWays := []maps.Way{}
	ways := offlineMaps.Ways()

	for i := range len(ways) {
		way := ways[i]
		onWay, err := way.OnWay(location, 2)
		if err != nil {
			slog.Debug("failed to check if on way", "error", err)
		}
		if onWay.OnWay {
			possibleWays = append(possibleWays, way)
		}
	}
	return possibleWays, nil
}

func NextWays(location log.GpsLocationData, currentWay CurrentWay, offlineMaps *maps.Offline, isForward bool) ([]maps.NextWayResult, error) {
	nextWays := []maps.NextWayResult{}
	dist := float32(0.0)
	wayIdx := currentWay.Way
	forward := isForward
	startPos := m.NewPosition(location.Latitude(), location.Longitude())
	for dist < ms.MIN_WAY_DIST {
		d, err := wayIdx.DistanceToEnd(startPos, forward)
		if err != nil || d <= 0 {
			break
		}
		dist += d
		nw, err := wayIdx.NextWay(offlineMaps, forward)
		if err != nil {
			break
		}
		nextWays = append(nextWays, nw)
		wayIdx = nw.Way

		startPos = nw.StartPosition
		forward = nw.IsForward
	}

	if len(nextWays) == 0 {
		nextWay, err := currentWay.Way.NextWay(offlineMaps, isForward)
		if err != nil {
			return []maps.NextWayResult{}, err
		}
		nextWays = append(nextWays, nextWay)
	}

	return nextWays, nil
}
