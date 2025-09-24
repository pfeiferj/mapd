package main

import (
	"math"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var MIN_WAY_DIST = 500 // meters. how many meters to look ahead before stopping gathering next ways.

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

// Bearing alignment thresholds
const ACCEPTABLE_BEARING_DELTA_SIN = 0.7071067811865475 // sin(45°) - max acceptable bearing mismatch

type OnWayResult struct {
	OnWay     bool
	Distance  DistanceResult
	IsForward bool
}

type WayCandidate struct {
	Way              Way
	OnWayResult      OnWayResult
	BearingAlignment float64 // sin(bearing_delta) - lower is better
	DistanceToWay    float64
	HierarchyRank    int
	Context          RoadContext
}

type DistanceResult struct {
	LineStart Coordinates
	LineEnd   Coordinates
	Distance  float64
}

// Updated CurrentWay struct with stability fields
type CurrentWay struct {
	Way               Way
	Distance          DistanceResult
	OnWay             OnWayResult
	StartPosition     Coordinates
	EndPosition       Coordinates
	ConfidenceCounter int
	LastChangeTime    time.Time
	StableDistance    float64
}

type NextWayResult struct {
	Way           Way
	IsForward     bool
	StartPosition Coordinates
	EndPosition   Coordinates
}

func OnWay(way Way, pos Position, extended bool) (OnWayResult, error) {
	res := OnWayResult{}
	if pos.Latitude < way.MaxLat()+PADDING && pos.Latitude > way.MinLat()-PADDING && pos.Longitude < way.MaxLon()+PADDING && pos.Longitude > way.MinLon()-PADDING {
		d, err := DistanceToWay(pos, way)
		res.Distance = d
		if err != nil {
			res.OnWay = false
			return res, errors.Wrap(err, "could not get distance to way")
		}
		lanes := way.Lanes()
		if lanes == 0 {
			lanes = 2
		}
		road_width_estimate := float64(lanes) * LANE_WIDTH
		max_dist := 5 + road_width_estimate
		if extended {
			max_dist = max_dist * 2
		}

		context := determineRoadContext(way, pos)
		if context == CONTEXT_FREEWAY {
			max_dist = max_dist * 1.5
		} else if context == CONTEXT_CITY {
			max_dist = max_dist * 0.8
		}

		if d.Distance < max_dist {
			res.OnWay = true
			res.IsForward = IsForward(d.LineStart, d.LineEnd, pos.Bearing)
			if !res.IsForward && way.OneWay() {
				res.OnWay = false
			}
			return res, nil
		}
	}
	res.OnWay = false
	return res, nil
}

func determineRoadContext(way Way, pos Position) RoadContext {
	lanes := way.Lanes()
	name, _ := way.Name()
	ref, _ := way.Ref()

	if isFreeway(way) || lanes >= 4 {
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

func isFreeway(way Way) bool {
	lanes := way.Lanes()
	name, _ := way.Name()
	ref, _ := way.Ref()

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
func getHighwayRank(way Way) int {
	name, _ := way.Name()
	ref, _ := way.Ref()
	lanes := way.Lanes()

	// Infer highway type from characteristics
	if isFreeway(way) {
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

func calculateBearingAlignment(way Way, pos Position) (float64, error) {
	d, err := DistanceToWay(pos, way)
	if err != nil {
		return 1.0, err
	}

	startLat := d.LineStart.Latitude()
	startLon := d.LineStart.Longitude()
	endLat := d.LineEnd.Latitude()
	endLon := d.LineEnd.Longitude()

	wayBearing := Bearing(startLat, startLon, endLat, endLon)

	// Calculate bearing delta
	delta := math.Abs(pos.Bearing*TO_RADIANS - wayBearing)

	// Normalize to 0-π range
	if delta > math.Pi {
		delta = 2*math.Pi - delta
	}
	return math.Sin(delta), nil
}

func selectBestWayAdvanced(possibleWays []Way, pos Position, currentWay Way, context RoadContext, gpsAccuracy float64) Way {
	if len(possibleWays) == 0 {
		return Way{}
	}
	if len(possibleWays) == 1 {
		return possibleWays[0]
	}

	bestWay := possibleWays[0]
	bestScore := float64(-1000)

	for _, way := range possibleWays {
		onWay, err := OnWay(way, pos, false)
		if err != nil || !onWay.OnWay {
			continue
		}

		score := float64(0)

		hierarchyRank := getHighwayRank(way)
		score += float64(100 - hierarchyRank)

		bearingAlignment, err := calculateBearingAlignment(way, pos)
		if err == nil {
			score += (1.0 - bearingAlignment) * 50
		}

		score -= onWay.Distance.Distance * 0.1

		if currentWay.HasNodes() {
			currentName, _ := currentWay.Name()
			currentRef, _ := currentWay.Ref()
			wayName, _ := way.Name()
			wayRef, _ := way.Ref()

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

// Legacy selectBestWay function for backward compatibility
func selectBestWay(possibleWays []Way, pos Position, currentWay Way, context RoadContext, currentStableDistance float64) Way {
	return selectBestWayAdvanced(possibleWays, pos, currentWay, context, 5.0)
}

func getRoadPriority(way Way, context RoadContext) int {
	lanes := way.Lanes()
	name, _ := way.Name()
	ref, _ := way.Ref()

	priority := LANE_COUNT_PRIORITY[lanes]
	if priority == 0 {
		priority = 30
	}

	switch context {
	case CONTEXT_FREEWAY:
		if isFreeway(way) {
			priority += 30
		}
		nameUpper := strings.ToUpper(name)
		if strings.Contains(nameUpper, "STREET") ||
			strings.Contains(nameUpper, "AVENUE") {
			priority -= 40
		}

	case CONTEXT_CITY:
		if isFreeway(way) {
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
		if isFreeway(way) {
			priority += 20
		}
	}

	if len(ref) > 0 {
		priority += 10
	}

	return priority
}

func DistanceToWay(pos Position, way Way) (DistanceResult, error) {
	res := DistanceResult{}
	var minNodeStart Coordinates
	var minNodeEnd Coordinates
	minDistance := math.MaxFloat64
	nodes, err := way.Nodes()
	if err != nil {
		return res, errors.Wrap(err, "could not read way nodes")
	}
	if nodes.Len() < 2 {
		return res, nil
	}

	latRad := pos.Latitude * TO_RADIANS
	lonRad := pos.Longitude * TO_RADIANS
	for i := 0; i < nodes.Len()-1; i++ {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		lineLat, lineLon := PointOnLine(nodeStart.Latitude(), nodeStart.Longitude(), nodeEnd.Latitude(), nodeEnd.Longitude(), pos.Latitude, pos.Longitude)
		distance := DistanceToPoint(latRad, lonRad, lineLat*TO_RADIANS, lineLon*TO_RADIANS)
		if distance < minDistance {
			minDistance = distance
			minNodeStart = nodeStart
			minNodeEnd = nodeEnd
		}
	}
	res.Distance = minDistance
	res.LineStart = minNodeStart
	res.LineEnd = minNodeEnd
	return res, nil
}

func GetWayStartEnd(way Way, isForward bool) (Coordinates, Coordinates) {
	if !way.HasNodes() {
		return Coordinates{}, Coordinates{}
	}

	nodes, err := way.Nodes()
	if err != nil {
		logde(errors.Wrap(err, "could not read way nodes"))
		return Coordinates{}, Coordinates{}
	}

	if nodes.Len() == 0 {
		return Coordinates{}, Coordinates{}
	}

	if nodes.Len() == 1 {
		return nodes.At(0), nodes.At(0)
	}

	if isForward {
		return nodes.At(0), nodes.At(nodes.Len() - 1)
	}
	return nodes.At(nodes.Len() - 1), nodes.At(0)
}

// distinguish between real turns and GPS noise
func isLikelyRealTurn(currentWay Way, newWay Way, pos Position, lastPos Position) bool {
	if !currentWay.HasNodes() || !newWay.HasNodes() {
		return true
	}

	// If GPS moved significantly and bearing changed significantly, it's likely a real turn
	if lastPos.Latitude != 0 && lastPos.Longitude != 0 {
		distance := DistanceToPoint(pos.Latitude*TO_RADIANS, pos.Longitude*TO_RADIANS,
			lastPos.Latitude*TO_RADIANS, lastPos.Longitude*TO_RADIANS)
		bearingChange := math.Abs(pos.Bearing - lastPos.Bearing)

		// Normalize bearing change to 0-180 degrees
		if bearingChange > math.Pi {
			bearingChange = 2*math.Pi - bearingChange
		}
		bearingChange = bearingChange * TO_DEGREES

		if distance > 10 && bearingChange > 20 {
			return true
		}

		// Very small movement with big bearing change = likely GPS noise
		if distance < 5 && bearingChange > 45 {
			return false
		}
	}

	currentName, _ := currentWay.Name()
	newName, _ := newWay.Name()
	currentRef, _ := currentWay.Ref()
	newRef, _ := newWay.Ref()

	if len(currentName) > 0 && len(newName) > 0 && currentName != newName {
		return true
	}
	if len(currentRef) > 0 && len(newRef) > 0 && currentRef != newRef {
		return true
	}

	if math.Abs(currentWay.MaxSpeed()-newWay.MaxSpeed()) > 10 {
		return true
	}

	return true
}

// Check if GPS position seems reasonable
func isGPSQualityGood(pos Position, lastPos Position) bool {
	if lastPos.Latitude == 0 || lastPos.Longitude == 0 {
		return true // First reading
	}

	// Check for GPS jumps (likely poor quality)
	distance := DistanceToPoint(pos.Latitude*TO_RADIANS, pos.Longitude*TO_RADIANS,
		lastPos.Latitude*TO_RADIANS, lastPos.Longitude*TO_RADIANS)

	// If GPS jumped more than 150m in 1 second, it's probably bad
	return distance < 150
}

func GetCurrentWay(currentWay CurrentWay, nextWays []NextWayResult, offline Offline, pos Position, lastPos Position, gpsAccuracy float64) (CurrentWay, error) {
	currentContext := CONTEXT_UNKNOWN
	if currentWay.Way.HasNodes() {
		currentContext = determineRoadContext(currentWay.Way, pos)
	}

	if currentWay.Way.HasNodes() {
		onWay, err := OnWay(currentWay.Way, pos, false)
		if err == nil && onWay.OnWay {
			stickThreshold := 15.0
			if currentContext == CONTEXT_FREEWAY {
				stickThreshold = 20.0
			} else if currentContext == CONTEXT_CITY {
				stickThreshold = 10.0
			}

			if onWay.Distance.Distance < stickThreshold {
				newStableDistance := onWay.Distance.Distance

				start, end := GetWayStartEnd(currentWay.Way, onWay.IsForward)
				return CurrentWay{
					Way:               currentWay.Way,
					Distance:          onWay.Distance,
					OnWay:             onWay,
					StartPosition:     start,
					EndPosition:       end,
					ConfidenceCounter: currentWay.ConfidenceCounter + 1,
					LastChangeTime:    currentWay.LastChangeTime,
					StableDistance:    newStableDistance,
				}, nil
			}
		}
	}

	for _, nextWay := range nextWays {
		onWay, err := OnWay(nextWay.Way, pos, false)
		if err == nil && onWay.OnWay {
			start, end := GetWayStartEnd(nextWay.Way, onWay.IsForward)
			return CurrentWay{
				Way:               nextWay.Way,
				Distance:          onWay.Distance,
				OnWay:             onWay,
				StartPosition:     start,
				EndPosition:       end,
				ConfidenceCounter: 1,
				LastChangeTime:    time.Now(),
				StableDistance:    onWay.Distance.Distance,
			}, nil
		}
	}

	possibleWays, err := getPossibleWays(offline, pos)
	if err == nil && len(possibleWays) > 0 {
		selectedWay := selectBestWayAdvanced(possibleWays, pos, currentWay.Way, currentContext, gpsAccuracy)
		if selectedWay.HasNodes() {
			selectedOnWay, err := OnWay(selectedWay, pos, false)
			if err == nil && selectedOnWay.OnWay {
				start, end := GetWayStartEnd(selectedWay, selectedOnWay.IsForward)
				return CurrentWay{
					Way:               selectedWay,
					Distance:          selectedOnWay.Distance,
					OnWay:             selectedOnWay,
					StartPosition:     start,
					EndPosition:       end,
					ConfidenceCounter: 1,
					LastChangeTime:    time.Now(),
					StableDistance:    selectedOnWay.Distance.Distance,
				}, nil
			}
		}
	}

	if currentWay.Way.HasNodes() {
		onWay, err := OnWay(currentWay.Way, pos, true)
		if err == nil && onWay.OnWay {
			start, end := GetWayStartEnd(currentWay.Way, onWay.IsForward)
			return CurrentWay{
				Way:               currentWay.Way,
				Distance:          onWay.Distance,
				OnWay:             onWay,
				StartPosition:     start,
				EndPosition:       end,
				ConfidenceCounter: currentWay.ConfidenceCounter,
				LastChangeTime:    currentWay.LastChangeTime,
				StableDistance:    currentWay.StableDistance,
			}, nil
		}
	}

	return CurrentWay{}, errors.New("could not find a current way")
}

func getPossibleWays(offline Offline, pos Position) ([]Way, error) {
	possibleWays := []Way{}
	ways, err := offline.Ways()
	if err != nil {
		return possibleWays, errors.Wrap(err, "could not get other ways")
	}

	for i := 0; i < ways.Len(); i++ {
		way := ways.At(i)
		onWay, err := OnWay(way, pos, false)
		logde(errors.Wrap(err, "Could not check if on way"))
		if onWay.OnWay {
			possibleWays = append(possibleWays, way)
		}
	}
	return possibleWays, nil
}

func IsForward(lineStart Coordinates, lineEnd Coordinates, bearing float64) bool {
	startLat := lineStart.Latitude()
	startLon := lineStart.Longitude()
	endLat := lineEnd.Latitude()
	endLon := lineEnd.Longitude()

	wayBearing := Bearing(startLat, startLon, endLat, endLon)
	bearingDelta := math.Abs(bearing*TO_RADIANS - wayBearing)
	return math.Cos(bearingDelta) >= 0
}

func MatchingWays(currentWay Way, offline Offline, matchNode Coordinates) ([]Way, error) {
	matchingWays := []Way{}
	ways, err := offline.Ways()
	if err != nil {
		return matchingWays, errors.Wrap(err, "could not read ways from offline")
	}

	for i := 0; i < ways.Len(); i++ {
		w := ways.At(i)
		if !w.HasNodes() {
			continue
		}

		if w.MinLat() == currentWay.MinLat() && w.MaxLat() == currentWay.MaxLat() && w.MinLon() == currentWay.MinLon() && w.MaxLon() == currentWay.MaxLon() {
			continue
		}

		wNodes, err := w.Nodes()
		if err != nil {
			return matchingWays, errors.Wrap(err, "could not read nodes from way")
		}
		if wNodes.Len() < 2 {
			continue
		}

		fNode := wNodes.At(0)
		lNode := wNodes.At(wNodes.Len() - 1)
		if (fNode.Latitude() == matchNode.Latitude() && fNode.Longitude() == matchNode.Longitude()) || (lNode.Latitude() == matchNode.Latitude() && lNode.Longitude() == matchNode.Longitude()) {
			matchingWays = append(matchingWays, w)
		}
	}

	return matchingWays, nil
}

func NextIsForward(nextWay Way, matchNode Coordinates) bool {
	if !nextWay.HasNodes() {
		return true
	}
	nodes, err := nextWay.Nodes()
	if err != nil || nodes.Len() < 2 {
		logde(errors.Wrap(err, "could not read next way nodes"))
		return true
	}

	lastNode := nodes.At(nodes.Len() - 1)
	if lastNode.Latitude() == matchNode.Latitude() && lastNode.Longitude() == matchNode.Longitude() {
		return false
	}

	return true
}

func NextWay(way Way, offline Offline, isForward bool) (NextWayResult, error) {
	nodes, err := way.Nodes()
	if err != nil {
		return NextWayResult{}, errors.Wrap(err, "could not read way nodes")
	}
	if !way.HasNodes() || nodes.Len() == 0 {
		return NextWayResult{}, nil
	}

	var matchNode Coordinates
	var matchBearingNode Coordinates
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

	if !PointInBox(matchNode.Latitude(), matchNode.Longitude(), offline.MinLat()-offline.Overlap(), offline.MinLon()-offline.Overlap(), offline.MaxLat()+offline.Overlap(), offline.MaxLon()+offline.Overlap()) {
		return NextWayResult{}, nil
	}

	matchingWays, err := MatchingWays(way, offline, matchNode)
	if err != nil {
		return NextWayResult{StartPosition: matchNode}, errors.Wrap(err, "could not check for next ways")
	}

	if len(matchingWays) == 0 {
		return NextWayResult{StartPosition: matchNode}, nil
	}

	context := determineRoadContext(way, Position{Latitude: matchNode.Latitude(), Longitude: matchNode.Longitude()})
	if context == CONTEXT_FREEWAY {
		filteredWays := []Way{}
		for _, mWay := range matchingWays {
			name, _ := mWay.Name()
			nameUpper := strings.ToUpper(name)
			if !strings.Contains(nameUpper, "SERVICE") &&
				!strings.Contains(nameUpper, "ACCESS") &&
				!(strings.Contains(nameUpper, "RAMP") && mWay.Lanes() < 2) {
				filteredWays = append(filteredWays, mWay)
			}
		}
		if len(filteredWays) > 0 {
			matchingWays = filteredWays
		}
	}

	curvatureThreshold := 0.15
	if context == CONTEXT_CITY {
		curvatureThreshold = 0.3
	} else if context == CONTEXT_FREEWAY {
		curvatureThreshold = 0.1
	}

	name, _ := way.Name()
	if len(name) > 0 {
		candidates := []Way{}
		for _, mWay := range matchingWays {
			mName, err := mWay.Name()
			if err != nil {
				continue
			}
			if mName == name {
				isForward := NextIsForward(mWay, matchNode)
				if !isForward && mWay.OneWay() {
					continue
				}

				if nodes.Len() > 1 && isValidConnection(mWay, matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, context)
			isForward := NextIsForward(bestWay, matchNode)
			start, end := GetWayStartEnd(bestWay, isForward)
			return NextWayResult{
				Way:           bestWay,
				StartPosition: start,
				EndPosition:   end,
				IsForward:     isForward,
			}, nil
		}
	}

	ref, _ := way.Ref()
	if len(ref) > 0 {
		candidates := []Way{}
		for _, mWay := range matchingWays {
			mRef, err := mWay.Ref()
			if err != nil {
				continue
			}
			if mRef == ref {
				isForward := NextIsForward(mWay, matchNode)
				if !isForward && mWay.OneWay() {
					continue
				}

				if nodes.Len() > 1 && isValidConnection(mWay, matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, context)
			isForward := NextIsForward(bestWay, matchNode)
			start, end := GetWayStartEnd(bestWay, isForward)
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
			mRef, err := mWay.Ref()
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
				if !isForward && mWay.OneWay() {
					continue
				}

				if nodes.Len() > 1 && isValidConnection(mWay, matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, context)
			isForward := NextIsForward(bestWay, matchNode)
			start, end := GetWayStartEnd(bestWay, isForward)
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
		if !isForward && mWay.OneWay() {
			continue
		}
		if nodes.Len() > 1 && isValidConnection(mWay, matchNode, matchBearingNode, curvatureThreshold) {
			validWays = append(validWays, mWay)
		}
	}

	if len(validWays) > 0 {
		bestWay := selectBestCandidate(validWays, matchNode, context)
		nextIsForward := NextIsForward(bestWay, matchNode)
		start, end := GetWayStartEnd(bestWay, nextIsForward)
		return NextWayResult{
			Way:           bestWay,
			StartPosition: start,
			EndPosition:   end,
			IsForward:     nextIsForward,
		}, nil
	}

	if len(matchingWays) > 0 {
		nextIsForward := NextIsForward(matchingWays[0], matchNode)
		start, end := GetWayStartEnd(matchingWays[0], nextIsForward)
		return NextWayResult{
			Way:           matchingWays[0],
			StartPosition: start,
			EndPosition:   end,
			IsForward:     nextIsForward,
		}, nil
	}

	return NextWayResult{StartPosition: matchNode}, nil
}

func isValidConnection(way Way, matchNode, bearingNode Coordinates, maxCurvature float64) bool {
	nodes, err := way.Nodes()
	if err != nil || nodes.Len() < 2 {
		return false
	}

	var nextBearingNode Coordinates
	if matchNode.Latitude() == nodes.At(0).Latitude() && matchNode.Longitude() == nodes.At(0).Longitude() {
		nextBearingNode = nodes.At(1)
	} else {
		nextBearingNode = nodes.At(nodes.Len() - 2)
	}

	curv, _, _ := GetCurvature(bearingNode.Latitude(), bearingNode.Longitude(), matchNode.Latitude(), matchNode.Longitude(), nextBearingNode.Latitude(), nextBearingNode.Longitude())
	return math.Abs(curv) <= maxCurvature
}

func selectBestCandidate(candidates []Way, matchNode Coordinates, context RoadContext) Way {
	if len(candidates) == 1 {
		return candidates[0]
	}

	bestWay := candidates[0]
	bestScore := float64(-1000)

	for _, way := range candidates {
		score := float64(getRoadPriority(way, context))

		lanes := way.Lanes()
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

func DistanceToEndOfWay(pos Position, way Way, isForward bool) (float64, error) {
	distanceResult, err := DistanceToWay(pos, way)
	if err != nil {
		return 0, err
	}
	lat := distanceResult.LineEnd.Latitude()
	lon := distanceResult.LineEnd.Longitude()
	dist := DistanceToPoint(pos.Latitude*TO_RADIANS, pos.Longitude*TO_RADIANS, lat*TO_RADIANS, lon*TO_RADIANS)
	stopFiltering := false
	nodes, err := way.Nodes()
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
		dist += DistanceToPoint(lat*TO_RADIANS, lon*TO_RADIANS, nLat*TO_RADIANS, nLon*TO_RADIANS)
		lat = nLat
		lon = nLon
	}
	return dist, nil
}

func NextWays(pos Position, currentWay CurrentWay, offline Offline, isForward bool) ([]NextWayResult, error) {
	nextWays := []NextWayResult{}
	dist := 0.0
	wayIdx := currentWay.Way
	forward := isForward
	startPos := pos
	for dist < float64(MIN_WAY_DIST) {
		d, err := DistanceToEndOfWay(startPos, wayIdx, forward)
		if err != nil || d <= 0 {
			break
		}
		dist += d
		nw, err := NextWay(wayIdx, offline, forward)
		if err != nil {
			break
		}
		nextWays = append(nextWays, nw)
		wayIdx = nw.Way
		startPos = Position{
			Latitude:  nw.StartPosition.Latitude(),
			Longitude: nw.StartPosition.Longitude(),
		}
		forward = nw.IsForward
	}

	if len(nextWays) == 0 {
		nextWay, err := NextWay(currentWay.Way, offline, isForward)
		if err != nil {
			return []NextWayResult{}, err
		}
		nextWays = append(nextWays, nextWay)
	}

	return nextWays, nil
}
