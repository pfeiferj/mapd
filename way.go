package main

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/cereal/offline"
	"pfeifer.dev/mapd/maps"
	ms "pfeifer.dev/mapd/settings"
)

type RoadContext int

const (
	CONTEXT_FREEWAY RoadContext = iota
	CONTEXT_CITY
	CONTEXT_UNKNOWN
)

// Road type detection and priorities
var LANE_COUNT_PRIORITY = map[uint8]int{
	8: 110, // Major freeway
	6: 100, // Freeway
	5: 95,
	4: 90, // Major arterial
	3: 70, // Arterial
	2: 50, // Collector/local
	1: 40, // Local street
	0: 30, // Unknown
}

// Highway hierarchy ranking
var HIGHWAY_RANK = map[string]int{
	"motorway":       0,
	"motorway_link":  1,
	"trunk":          10,
	"trunk_link":     11,
	"primary":        20,
	"primary_link":   21,
	"secondary":      30,
	"secondary_link": 31,
	"tertiary":       40,
	"tertiary_link":  41,
	"unclassified":   50,
	"residential":    60,
	"living_street":  61,
}

type OnWayResult struct {
	OnWay     bool
	Distance  DistanceResult
	IsForward bool
}

type WayCandidate struct {
	Way              offline.Way
	OnWayResult      OnWayResult
	BearingAlignment float32 // sin(bearing_delta) - lower is better
	DistanceToWay    float32
	HierarchyRank    int
	Context          RoadContext
}

type DistanceResult struct {
	LineStart      offline.Coordinates
	LineEnd        offline.Coordinates
	LinePoint      LinePoint
	Distance       float32
}

// Updated CurrentWay struct with stability fields
type CurrentWay struct {
	Way               Way
	Distance          DistanceResult
	OnWay             OnWayResult
	StartPosition     offline.Coordinates
	EndPosition       offline.Coordinates
	ConfidenceCounter int
	LastChangeTime    time.Time
	StableDistance    float32
	SelectionType     custom.WaySelectionType
}

type NextWayResult struct {
	Way           Way
	IsForward     bool
	StartPosition offline.Coordinates
	EndPosition   offline.Coordinates
}

type Way struct {
	Way offline.Way
	Width float32
	Context RoadContext
	IsFreeway bool
	Name string
	Distance float32
	Rank int
	Priority int
	DistanceMultiplier float32
}

func (w *Way) Init() {
	var err error
	w.Distance, err = w.distance()
	if err != nil {
		slog.Warn("could not calculate way distance", "error", err)
	}
	w.Name = w.roadName()
	w.Width = w.roadWidth()
	w.IsFreeway = w.isFreeway()
	w.Context = w.context()
	w.Rank = w.rank()
	w.Priority = w.getPriority()
	w.DistanceMultiplier = w.distanceMultiplier()
}

func (w *Way) OnWay(location log.GpsLocationData, distanceMultiplier float32) (OnWayResult, error) {
	res := OnWayResult{}
	d, err := w.DistanceFrom(location.Latitude(), location.Longitude())
	res.Distance = d
	if err != nil {
		res.OnWay = false
		return res, errors.Wrap(err, "could not get distance to way")
	}
	max_dist := max(location.HorizontalAccuracy(), 5) + w.Width
	max_dist *= distanceMultiplier

	if d.Distance < max_dist {
		res.OnWay = true
		res.IsForward = IsForward(d.LineStart, d.LineEnd, float64(location.BearingDeg()))
		if !res.IsForward && w.Way.OneWay() {
			res.OnWay = false
		}
		return res, nil
	}
	res.OnWay = false
	return res, nil
}


func (w *Way) bearingAlignment(location log.GpsLocationData) (float32, error) {
	d, err := w.DistanceFrom(location.Latitude(), location.Longitude())
	if err != nil {
		return 1.0, err
	}

	startLat := d.LineStart.Latitude()
	startLon := d.LineStart.Longitude()
	endLat := d.LineEnd.Latitude()
	endLon := d.LineEnd.Longitude()

	wayBearing := Bearing(startLat, startLon, endLat, endLon)

	// Calculate bearing delta
	delta := math.Abs(float64(location.BearingDeg())*ms.TO_RADIANS - wayBearing)

	// Normalize to 0-π range
	if delta > math.Pi {
		delta = 2*math.Pi - delta
	}
	return float32(math.Sin(delta)), nil
}

func selectBestWayAdvanced(possibleWays []Way, location log.GpsLocationData, currentWay Way) Way {
	if len(possibleWays) == 0 {
		return Way{}
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

		score += float32(100 - way.Rank)

		bearingAlignment, err := way.bearingAlignment(location)
		if err == nil {
			score += (1.0 - bearingAlignment) * 50
		}

		score -= onWay.Distance.Distance * 0.1

		if currentWay.Way.HasNodes() {
			currentName, _ := currentWay.Way.Name()
			currentRef, _ := currentWay.Way.Ref()
			wayName, _ := way.Way.Name()
			wayRef, _ := way.Way.Ref()

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

func (w *Way) DistanceFrom(latitude float64, longitude float64) (DistanceResult, error) {
	res := DistanceResult{}
	var minNodeStart offline.Coordinates
	var minNodeEnd offline.Coordinates
	minDistance := float32(math.MaxFloat32)
	nodes, err := w.Way.Nodes()
	if err != nil {
		return res, errors.Wrap(err, "could not read way nodes")
	}
	if nodes.Len() < 2 {
		return res, errors.Wrap(err, "not enough nodes to determine distance")
	}

	latRad := latitude * ms.TO_RADIANS
	lonRad := longitude * ms.TO_RADIANS
	minLinePoint := LinePoint{}
	minIdx := 0
	for i := 0; i < nodes.Len()-1; i++ {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		linePoint := PointOnLine(nodeStart.Latitude(), nodeStart.Longitude(), nodeEnd.Latitude(), nodeEnd.Longitude(), latitude, longitude)

		distance := DistanceToPoint(latRad, lonRad, linePoint.X*ms.TO_RADIANS, linePoint.Y*ms.TO_RADIANS)
		if distance < minDistance {
			minDistance = distance
			minNodeStart = nodeStart
			minNodeEnd = nodeEnd
			minLinePoint = linePoint
			minIdx = i
		}
	}
	onWayDistance := DistanceToPoint(minNodeStart.Latitude()*ms.TO_RADIANS, lonRad*ms.TO_RADIANS, minLinePoint.X*ms.TO_RADIANS, minLinePoint.Y*ms.TO_RADIANS)
	for i := range minIdx {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		onWayDistance += DistanceToPoint(nodeStart.Latitude()*ms.TO_RADIANS, nodeStart.Longitude()*ms.TO_RADIANS, nodeEnd.Latitude()*ms.TO_RADIANS, nodeEnd.Longitude()*ms.TO_RADIANS)
	}

	res.Distance = minDistance
	res.LineStart = minNodeStart
	res.LineEnd = minNodeEnd
	res.LinePoint = minLinePoint
	return res, nil
}

func (w *Way) GetStartEnd(isForward bool) (offline.Coordinates, offline.Coordinates) {
	if !w.Way.HasNodes() {
		return offline.Coordinates{}, offline.Coordinates{}
	}

	nodes, err := w.Way.Nodes()
	if err != nil {
		slog.Debug("could not read way nodes", "error", err)
		return offline.Coordinates{}, offline.Coordinates{}
	}

	if nodes.Len() == 0 {
		return offline.Coordinates{}, offline.Coordinates{}
	}

	if nodes.Len() == 1 {
		return nodes.At(0), nodes.At(0)
	}

	if isForward {
		return nodes.At(0), nodes.At(nodes.Len() - 1)
	}
	return nodes.At(nodes.Len() - 1), nodes.At(0)
}

func GetCurrentWay(currentWay CurrentWay, nextWays []NextWayResult, offline offline.Offline, location log.GpsLocationData) (CurrentWay, error) {
	distanceFromCurrentWay := currentWay.OnWay.Distance.Distance
	nodes, err := currentWay.Way.Way.Nodes()
	if err == nil && nodes.Len() > 1 {
		onWay, err := currentWay.Way.OnWay(location, currentWay.Way.DistanceMultiplier)
		newStableDistance := onWay.Distance.Distance
		distanceFromCurrentWay = newStableDistance
		t := onWay.Distance.LinePoint.T
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
		if !nextWay.Way.Way.HasNodes() {
			continue
		}
		onWay, err := nextWay.Way.OnWay(location, nextWay.Way.DistanceMultiplier)
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
		if selectedWay.Way.HasNodes() {
			selectedOnWay, err := selectedWay.OnWay(location, selectedWay.DistanceMultiplier)
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

	if currentWay.Way.Way.HasNodes() {
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

func getPossibleWays(offlineMaps offline.Offline, location log.GpsLocationData) ([]Way, error) {
	possibleWays := []Way{}
	ways, err := offlineMaps.Ways()
	if err != nil {
		return possibleWays, errors.Wrap(err, "could not get other ways")
	}

	for i := 0; i < ways.Len(); i++ {
		way := ways.At(i)
		w := Way{Way: way}
		w.Init()
		onWay, err := w.OnWay(location, 2)
		if err != nil {
			slog.Debug("failed to check if on way", "error", err)
		}
		if onWay.OnWay {
			possibleWays = append(possibleWays, w)
		}
	}
	return possibleWays, nil
}

func IsForward(lineStart offline.Coordinates, lineEnd offline.Coordinates, bearing float64) bool {
	startLat := lineStart.Latitude()
	startLon := lineStart.Longitude()
	endLat := lineEnd.Latitude()
	endLon := lineEnd.Longitude()

	wayBearing := Bearing(startLat, startLon, endLat, endLon)
	bearingDelta := math.Abs(bearing*ms.TO_RADIANS - wayBearing)
	return math.Cos(bearingDelta) >= 0
}

func (w *Way) MatchingWays(offlineMaps offline.Offline, matchNode offline.Coordinates) ([]Way, error) {
	matchingWays := []Way{}
	ways, err := offlineMaps.Ways()
	if err != nil {
		return matchingWays, errors.Wrap(err, "could not read ways from offline")
	}

	for i := 0; i < ways.Len(); i++ {
		way := ways.At(i)
		if !way.HasNodes() {
			continue
		}

		if way.MinLat() == w.Way.MinLat() && way.MaxLat() == w.Way.MaxLat() && way.MinLon() == w.Way.MinLon() && way.MaxLon() == w.Way.MaxLon() {
			continue
		}

		wNodes, err := way.Nodes()
		if err != nil {
			return matchingWays, errors.Wrap(err, "could not read nodes from way")
		}
		if wNodes.Len() < 2 {
			continue
		}

		fNode := wNodes.At(0)
		lNode := wNodes.At(wNodes.Len() - 1)
		if (fNode.Latitude() == matchNode.Latitude() && fNode.Longitude() == matchNode.Longitude()) || (lNode.Latitude() == matchNode.Latitude() && lNode.Longitude() == matchNode.Longitude()) {
			w := Way{Way: way}
			w.Init()
			matchingWays = append(matchingWays, w)
		}
	}

	return matchingWays, nil
}

func NextIsForward(nextWay Way, matchNode offline.Coordinates) bool {
	if !nextWay.Way.HasNodes() {
		return true
	}
	nodes, err := nextWay.Way.Nodes()
	if err != nil || nodes.Len() < 2 {
		if err != nil {
			slog.Debug("could not read next way nodes", "error", err)
		}
		return true
	}

	lastNode := nodes.At(nodes.Len() - 1)
	if lastNode.Latitude() == matchNode.Latitude() && lastNode.Longitude() == matchNode.Longitude() {
		return false
	}

	return true
}

func (w *Way) NextWay(offlineMaps offline.Offline, isForward bool) (NextWayResult, error) {
	nodes, err := w.Way.Nodes()
	if err != nil {
		return NextWayResult{}, errors.Wrap(err, "could not read way nodes")
	}
	if !w.Way.HasNodes() || nodes.Len() == 0 {
		return NextWayResult{}, nil
	}

	var matchNode offline.Coordinates
	var matchBearingNode offline.Coordinates
	if isForward {
		matchNode = nodes.At(nodes.Len() - 1)
		if nodes.Len() > 1 {
			matchBearingNode = nodes.At(nodes.Len() - 2)
		}
	} else {
		matchNode = nodes.At(0)
		if nodes.Len() > 1 {
			matchBearingNode = nodes.At(1)
		}
	}

	if !maps.PointInBox(matchNode.Latitude(), matchNode.Longitude(), offlineMaps.MinLat()-offlineMaps.Overlap(), offlineMaps.MinLon()-offlineMaps.Overlap(), offlineMaps.MaxLat()+offlineMaps.Overlap(), offlineMaps.MaxLon()+offlineMaps.Overlap()) {
		return NextWayResult{}, nil
	}

	matchingWays, err := w.MatchingWays(offlineMaps, matchNode)
	if err != nil {
		return NextWayResult{StartPosition: matchNode}, errors.Wrap(err, "could not check for next ways")
	}

	if len(matchingWays) == 0 {
		return NextWayResult{StartPosition: matchNode}, nil
	}

	if w.Context == CONTEXT_FREEWAY {
		filteredWays := []Way{}
		for _, mWay := range matchingWays {
			name, _ := mWay.Way.Name()
			nameUpper := strings.ToUpper(name)
			if !strings.Contains(nameUpper, "SERVICE") &&
				!strings.Contains(nameUpper, "ACCESS") &&
				!(strings.Contains(nameUpper, "RAMP") && mWay.Way.Lanes() < 2) {
				filteredWays = append(filteredWays, mWay)
			}
		}
		if len(filteredWays) > 0 {
			matchingWays = filteredWays
		}
	}

	curvatureThreshold := 0.15
	if w.Context == CONTEXT_CITY {
		curvatureThreshold = 0.3
	} else if w.Context == CONTEXT_FREEWAY {
		curvatureThreshold = 0.1
	}

	name, _ := w.Way.Name()
	if len(name) > 0 {
		candidates := []Way{}
		for _, mWay := range matchingWays {
			mName, err := mWay.Way.Name()
			if err != nil {
				continue
			}
			if mName == name {
				isForward := NextIsForward(mWay, matchNode)
				if !isForward && mWay.Way.OneWay() {
					continue
				}

				if nodes.Len() > 1 && mWay.isValidConnection(matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, w.Context)
			isForward := NextIsForward(bestWay, matchNode)
			start, end := bestWay.GetStartEnd(isForward)
			return NextWayResult{
				Way:           bestWay,
				StartPosition: start,
				EndPosition:   end,
				IsForward:     isForward,
			}, nil
		}
	}

	ref, _ := w.Way.Ref()
	if len(ref) > 0 {
		candidates := []Way{}
		for _, mWay := range matchingWays {
			mRef, err := mWay.Way.Ref()
			if err != nil {
				continue
			}
			if mRef == ref {
				isForward := NextIsForward(mWay, matchNode)
				if !isForward && mWay.Way.OneWay() {
					continue
				}

				if nodes.Len() > 1 && mWay.isValidConnection(matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, w.Context)
			isForward := NextIsForward(bestWay, matchNode)
			start, end := bestWay.GetStartEnd(isForward)
			return NextWayResult{
				Way:           bestWay,
				StartPosition: start,
				EndPosition:   end,
				IsForward:     isForward,
			}, nil
		}
	}

	if len(ref) > 0 {
		refs := strings.Split(ref, ";")
		candidates := []Way{}
		for _, mWay := range matchingWays {
			mRef, err := mWay.Way.Ref()
			if err != nil {
				continue
			}
			mRefs := strings.Split(mRef, ";")
			hasMatch := false
			for _, r := range refs {
				for _, mr := range mRefs {
					hasMatch = hasMatch || (strings.TrimSpace(r) == strings.TrimSpace(mr))
				}
			}
			if hasMatch {
				isForward := NextIsForward(mWay, matchNode)
				if !isForward && mWay.Way.OneWay() {
					continue
				}

				if nodes.Len() > 1 && mWay.isValidConnection(matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, w.Context)
			isForward := NextIsForward(bestWay, matchNode)
			start, end := bestWay.GetStartEnd(isForward)
			return NextWayResult{
				Way:           bestWay,
				StartPosition: start,
				EndPosition:   end,
				IsForward:     isForward,
			}, nil
		}
	}

	validWays := []Way{}
	for _, mWay := range matchingWays {
		isForward := NextIsForward(mWay, matchNode)
		if !isForward && mWay.Way.OneWay() {
			continue
		}
		if nodes.Len() > 1 && mWay.isValidConnection(matchNode, matchBearingNode, curvatureThreshold) {
			validWays = append(validWays, mWay)
		}
	}

	if len(validWays) > 0 {
		bestWay := selectBestCandidate(validWays, matchNode, w.Context)
		nextIsForward := NextIsForward(bestWay, matchNode)
		start, end := bestWay.GetStartEnd(nextIsForward)
		return NextWayResult{
			Way:           bestWay,
			StartPosition: start,
			EndPosition:   end,
			IsForward:     nextIsForward,
		}, nil
	}

	if len(matchingWays) > 0 {
		nextIsForward := NextIsForward(matchingWays[0], matchNode)
		start, end := matchingWays[0].GetStartEnd(nextIsForward)
		return NextWayResult{
			Way:           matchingWays[0],
			StartPosition: start,
			EndPosition:   end,
			IsForward:     nextIsForward,
		}, nil
	}

	return NextWayResult{StartPosition: matchNode}, nil
}

func (w *Way) isValidConnection(matchNode, bearingNode offline.Coordinates, maxCurvature float64) bool {
	nodes, err := w.Way.Nodes()
	if err != nil || nodes.Len() < 2 {
		return false
	}

	var nextBearingNode offline.Coordinates
	if matchNode.Latitude() == nodes.At(0).Latitude() && matchNode.Longitude() == nodes.At(0).Longitude() {
		nextBearingNode = nodes.At(1)
	} else {
		nextBearingNode = nodes.At(nodes.Len() - 2)
	}

	curv, _, _ := GetCurvature(bearingNode.Latitude(), bearingNode.Longitude(), matchNode.Latitude(), matchNode.Longitude(), nextBearingNode.Latitude(), nextBearingNode.Longitude())
	return math.Abs(curv) <= maxCurvature
}

func selectBestCandidate(candidates []Way, matchNode offline.Coordinates, context RoadContext) Way {
	if len(candidates) == 1 {
		return candidates[0]
	}

	bestWay := candidates[0]
	bestScore := float64(-1000)

	for _, way := range candidates {
		score := float64(way.Priority)

		lanes := way.Way.Lanes()
		if lanes > 0 {
			laneWeight := 2.0
			if context == CONTEXT_FREEWAY {
				laneWeight = 4.0
			} else if context == CONTEXT_CITY {
				laneWeight = 1.0
			}
			score += float64(lanes) * laneWeight
		}

		if score > bestScore {
			bestScore = score
			bestWay = way
		}
	}

	return bestWay
}

func (w *Way) DistanceToEnd(latitude float64, longitude float64, isForward bool) (float32, error) {
	distanceResult, err := w.DistanceFrom(latitude, longitude)
	if err != nil {
		return 0, err
	}
	lat := distanceResult.LineEnd.Latitude()
	lon := distanceResult.LineEnd.Longitude()
	dist := DistanceToPoint(latitude*ms.TO_RADIANS, longitude*ms.TO_RADIANS, lat*ms.TO_RADIANS, lon*ms.TO_RADIANS)
	stopFiltering := false
	nodes, err := w.Way.Nodes()
	if err != nil {
		return 0, err
	}
	for i := 0; i < nodes.Len(); i++ {
		index := i
		if !isForward {
			index = nodes.Len() - 1 - i
		}
		node := nodes.At(index)
		nLat := node.Latitude()
		nLon := node.Longitude()
		if node.Latitude() == lat && node.Longitude() == lon && !stopFiltering {
			stopFiltering = true
		}
		if !stopFiltering {
			continue
		}
		dist += DistanceToPoint(lat*ms.TO_RADIANS, lon*ms.TO_RADIANS, nLat*ms.TO_RADIANS, nLon*ms.TO_RADIANS)
		lat = nLat
		lon = nLon
	}
	return dist, nil
}

func NextWays(location log.GpsLocationData, currentWay CurrentWay, offlineMaps offline.Offline, isForward bool) ([]NextWayResult, error) {
	nextWays := []NextWayResult{}
	dist := float32(0.0)
	wayIdx := currentWay.Way
	forward := isForward
	startLat := location.Latitude()
	startLon := location.Longitude()
	for dist < ms.MIN_WAY_DIST {
		d, err := wayIdx.DistanceToEnd(startLat, startLon, forward)
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

		startLat = nw.StartPosition.Latitude()
		startLon = nw.StartPosition.Longitude()
		forward = nw.IsForward
	}

	if len(nextWays) == 0 {
		nextWay, err := currentWay.Way.NextWay(offlineMaps, isForward)
		if err != nil {
			return []NextWayResult{}, err
		}
		nextWays = append(nextWays, nextWay)
	}

	return nextWays, nil
}

func (w *Way) distance() (float32, error) {
	nodes, err := w.Way.Nodes()
	if err != nil {
		return 0, err
	}

	if nodes.Len() < 2 {
		return 0, nil
	}

	totalDistance := float32(0.0)
	for i := range nodes.Len() - 1 {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		distance := DistanceToPoint(
			nodeStart.Latitude()*ms.TO_RADIANS,
			nodeStart.Longitude()*ms.TO_RADIANS,
			nodeEnd.Latitude()*ms.TO_RADIANS,
			nodeEnd.Longitude()*ms.TO_RADIANS,
		)
		totalDistance += distance
	}

	return totalDistance, nil
}

func (w *Way) roadName() string {
	name, err := w.Way.Name()
	if err == nil {
		if len(name) > 0 {
			return name
		}
	}
	ref, err := w.Way.Ref()
	if err == nil {
		if len(ref) > 0 {
			return ref
		}
	}
	return ""
}

func (w *Way) roadWidth() float32 {
	lanes := w.Way.Lanes()
	if lanes == 0 {
		lanes = 2
	}
	return float32(lanes) * ms.Settings.DefaultLaneWidth
}

func (w *Way) context() RoadContext {
	lanes := w.Way.Lanes()
	name, _ := w.Way.Name()
	ref, _ := w.Way.Ref()

	if w.IsFreeway || lanes >= 4 {
		return CONTEXT_FREEWAY
	}

	nameUpper := strings.ToUpper(name)
	if lanes <= 3 && (strings.Contains(nameUpper, "STREET") ||
		strings.Contains(nameUpper, "AVENUE") ||
		strings.Contains(nameUpper, "BOULEVARD") ||
		strings.Contains(nameUpper, "ROAD") ||
		len(ref) == 0) {
		return CONTEXT_CITY
	}

	return CONTEXT_UNKNOWN
}

func (w *Way) isFreeway() bool {
	lanes := w.Way.Lanes()
	name, _ := w.Way.Name()
	ref, _ := w.Way.Ref()

	if lanes >= 6 {
		return true
	}

	nameUpper := strings.ToUpper(name)
	refUpper := strings.ToUpper(ref)

	if strings.Contains(nameUpper, "INTERSTATE") ||
		strings.Contains(nameUpper, "FREEWAY") ||
		strings.Contains(nameUpper, "EXPRESSWAY") ||
		strings.Contains(nameUpper, "PARKWAY") ||
		strings.HasPrefix(refUpper, "I-") ||
		strings.HasPrefix(refUpper, "I ") ||
		(lanes >= 4 && len(ref) > 0 && !strings.Contains(nameUpper, "STREET")) {
		return true
	}

	return false
}

// Get highway hierarchy rank for a way
func (w *Way) rank() int {
	name, _ := w.Way.Name()
	ref, _ := w.Way.Ref()
	lanes := w.Way.Lanes()

	// Infer highway type from characteristics
	if w.IsFreeway {
		if lanes >= 6 {
			return HIGHWAY_RANK["motorway"]
		}
		return HIGHWAY_RANK["trunk"]
	}

	nameUpper := strings.ToUpper(name)
	refUpper := strings.ToUpper(ref)

	// Primary roads (usually have ref numbers)
	if len(ref) > 0 && !strings.Contains(nameUpper, "STREET") {
		if strings.HasPrefix(refUpper, "US-") || strings.HasPrefix(refUpper, "SR-") {
			return HIGHWAY_RANK["primary"]
		}
		return HIGHWAY_RANK["secondary"]
	}

	// Local roads
	if strings.Contains(nameUpper, "STREET") ||
		strings.Contains(nameUpper, "AVENUE") ||
		strings.Contains(nameUpper, "ROAD") {
		return HIGHWAY_RANK["residential"]
	}

	// Default to unclassified
	return HIGHWAY_RANK["unclassified"]
}

func (w *Way) getPriority() int {
	lanes := w.Way.Lanes()
	name, _ := w.Way.Name()
	ref, _ := w.Way.Ref()

	priority := LANE_COUNT_PRIORITY[lanes]
	if priority == 0 {
		priority = 30
	}

	switch w.Context {
	case CONTEXT_FREEWAY:
		if w.IsFreeway {
			priority += 30
		}
		nameUpper := strings.ToUpper(name)
		if strings.Contains(nameUpper, "STREET") ||
			strings.Contains(nameUpper, "AVENUE") {
			priority -= 40
		}

	case CONTEXT_CITY:
		if w.IsFreeway {
			priority += 10
		}
		nameUpper := strings.ToUpper(name)
		if strings.Contains(nameUpper, "SERVICE") {
			priority -= 5
		}
		// Boost local street names in city context
		if strings.Contains(nameUpper, "STREET") ||
			strings.Contains(nameUpper, "AVENUE") ||
			strings.Contains(nameUpper, "ROAD") {
			priority += 5
		}

	case CONTEXT_UNKNOWN:
		if w.IsFreeway {
			priority += 20
		}
	}

	if len(ref) > 0 {
		priority += 10
	}

	return priority
}

func (w *Way) distanceMultiplier() float32 {
	switch w.Context {
	case CONTEXT_CITY:
		return 0.75
	case CONTEXT_FREEWAY:
		return 1.5
	default:
		return 1
	}
}
