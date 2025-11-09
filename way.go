package main

import (
	"math"
	"strings"
	"time"

	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/cereal/offline"
	"pfeifer.dev/mapd/maps"
	ms "pfeifer.dev/mapd/settings"
	"pfeifer.dev/mapd/utils"
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
	Way              offline.Way
	OnWayResult      OnWayResult
	BearingAlignment float64 // sin(bearing_delta) - lower is better
	DistanceToWay    float64
	HierarchyRank    int
	Context          RoadContext
}

type DistanceResult struct {
	LineStart      offline.Coordinates
	LineEnd        offline.Coordinates
	LineLat        float64
	LineLon        float64
	Distance       float64
	DistanceOnPath float64
}

// Updated CurrentWay struct with stability fields
type CurrentWay struct {
	Way               offline.Way
	Distance          DistanceResult
	OnWay             OnWayResult
	StartPosition     offline.Coordinates
	EndPosition       offline.Coordinates
	ConfidenceCounter int
	LastChangeTime    time.Time
	StableDistance    float64
	Context           RoadContext
}

type NextWayResult struct {
	Way           offline.Way
	IsForward     bool
	StartPosition offline.Coordinates
	EndPosition   offline.Coordinates
}

func estimateRoadWidth(way offline.Way) float64 {
	lanes := way.Lanes()
	if lanes == 0 {
		lanes = 2
	}
	return float64(lanes) * float64(ms.Settings.DefaultLaneWidth)
}

func OnWay(way offline.Way, location log.GpsLocationData, extended bool) (OnWayResult, error) {
	res := OnWayResult{}
	if location.Latitude() < way.MaxLat()+ms.PADDING && location.Latitude() > way.MinLat()-ms.PADDING && location.Longitude() < way.MaxLon()+ms.PADDING && location.Longitude() > way.MinLon()-ms.PADDING {
		d, err := DistanceToWay(location.Latitude(), location.Longitude(), way)
		res.Distance = d
		if err != nil {
			res.OnWay = false
			return res, errors.Wrap(err, "could not get distance to way")
		}
		road_width_estimate := estimateRoadWidth(way)
		max_dist := 5 + road_width_estimate
		if extended {
			max_dist = max_dist * 2
		}

		context := determineRoadContext(way)
		if context == CONTEXT_FREEWAY {
			max_dist = max_dist * 1.5
		} else if context == CONTEXT_CITY {
			max_dist = max_dist * 0.8
		}

		if d.Distance < max_dist {
			res.OnWay = true
			res.IsForward = IsForward(d.LineStart, d.LineEnd, float64(location.BearingDeg()))
			if !res.IsForward && way.OneWay() {
				res.OnWay = false
			}
			return res, nil
		}
	}
	res.OnWay = false
	return res, nil
}

func determineRoadContext(way offline.Way) RoadContext {
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

func isFreeway(way offline.Way) bool {
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
func getHighwayRank(way offline.Way) int {
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

func calculateBearingAlignment(way offline.Way, location log.GpsLocationData) (float64, error) {
	d, err := DistanceToWay(location.Latitude(), location.Longitude(), way)
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
	return math.Sin(delta), nil
}

func selectBestWayAdvanced(possibleWays []offline.Way, location log.GpsLocationData, currentWay offline.Way, context RoadContext) offline.Way {
	if len(possibleWays) == 0 {
		return offline.Way{}
	}
	if len(possibleWays) == 1 {
		return possibleWays[0]
	}

	bestWay := possibleWays[0]
	bestScore := float64(-1000)

	for _, way := range possibleWays {
		onWay, err := OnWay(way, location, false)
		if err != nil || !onWay.OnWay {
			continue
		}

		score := float64(0)

		hierarchyRank := getHighwayRank(way)
		score += float64(100 - hierarchyRank)

		bearingAlignment, err := calculateBearingAlignment(way, location)
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

func getRoadPriority(way offline.Way, context RoadContext) int {
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

func DistanceToWay(latitude float64, longitude float64, way offline.Way) (DistanceResult, error) {
	res := DistanceResult{}
	var minNodeStart offline.Coordinates
	var minNodeEnd offline.Coordinates
	minDistance := math.MaxFloat64
	nodes, err := way.Nodes()
	if err != nil {
		return res, errors.Wrap(err, "could not read way nodes")
	}
	if nodes.Len() < 2 {
		return res, nil
	}

	latRad := latitude * ms.TO_RADIANS
	lonRad := longitude * ms.TO_RADIANS
	minLineLat := latitude
	minLineLon := longitude
	minIdx := 0
	for i := 0; i < nodes.Len()-1; i++ {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		lineLat, lineLon := PointOnLine(nodeStart.Latitude(), nodeStart.Longitude(), nodeEnd.Latitude(), nodeEnd.Longitude(), latitude, longitude)
		distance := DistanceToPoint(latRad, lonRad, lineLat*ms.TO_RADIANS, lineLon*ms.TO_RADIANS)
		if distance < minDistance {
			minDistance = distance
			minNodeStart = nodeStart
			minNodeEnd = nodeEnd
			minLineLat = lineLat
			minLineLon = lineLon
			minIdx = i
		}
	}
	onWayDistance := DistanceToPoint(minNodeStart.Latitude()*ms.TO_RADIANS, lonRad*ms.TO_RADIANS, minLineLat*ms.TO_RADIANS, minLineLon*ms.TO_RADIANS)
	for i := range minIdx {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		onWayDistance += DistanceToPoint(nodeStart.Latitude()*ms.TO_RADIANS, nodeStart.Longitude()*ms.TO_RADIANS, nodeEnd.Latitude()*ms.TO_RADIANS, nodeEnd.Longitude()*ms.TO_RADIANS)
	}

	res.Distance = minDistance
	res.LineStart = minNodeStart
	res.LineEnd = minNodeEnd
	res.LineLat = minLineLat
	res.LineLon = minLineLon
	return res, nil
}

func GetWayStartEnd(way offline.Way, isForward bool) (offline.Coordinates, offline.Coordinates) {
	if !way.HasNodes() {
		return offline.Coordinates{}, offline.Coordinates{}
	}

	nodes, err := way.Nodes()
	if err != nil {
		utils.Logde(errors.Wrap(err, "could not read way nodes"))
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
	currentContext := CONTEXT_UNKNOWN
	if currentWay.Way.HasNodes() {
		currentContext = determineRoadContext(currentWay.Way)
	}

	if currentWay.Way.HasNodes() {
		onWay, err := OnWay(currentWay.Way, location, false)
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
					Context:           determineRoadContext(currentWay.Way),
				}, nil
			}
		}
	}

	for _, nextWay := range nextWays {
		onWay, err := OnWay(nextWay.Way, location, false)
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
				Context:           determineRoadContext(nextWay.Way),
			}, nil
		}
	}

	possibleWays, err := getPossibleWays(offline, location)
	if err == nil && len(possibleWays) > 0 {
		selectedWay := selectBestWayAdvanced(possibleWays, location, currentWay.Way, currentContext)
		if selectedWay.HasNodes() {
			selectedOnWay, err := OnWay(selectedWay, location, false)
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
					Context:           determineRoadContext(selectedWay),
				}, nil
			}
		}
	}

	if currentWay.Way.HasNodes() {
		onWay, err := OnWay(currentWay.Way, location, true)
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
				Context:           determineRoadContext(currentWay.Way),
			}, nil
		}
	}

	return CurrentWay{}, errors.New("could not find a current way")
}

func getPossibleWays(offlineMaps offline.Offline, location log.GpsLocationData) ([]offline.Way, error) {
	possibleWays := []offline.Way{}
	ways, err := offlineMaps.Ways()
	if err != nil {
		return possibleWays, errors.Wrap(err, "could not get other ways")
	}

	for i := 0; i < ways.Len(); i++ {
		way := ways.At(i)
		onWay, err := OnWay(way, location, false)
		utils.Logde(errors.Wrap(err, "Could not check if on way"))
		if onWay.OnWay {
			possibleWays = append(possibleWays, way)
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

func MatchingWays(currentWay offline.Way, offlineMaps offline.Offline, matchNode offline.Coordinates) ([]offline.Way, error) {
	matchingWays := []offline.Way{}
	ways, err := offlineMaps.Ways()
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

func NextIsForward(nextWay offline.Way, matchNode offline.Coordinates) bool {
	if !nextWay.HasNodes() {
		return true
	}
	nodes, err := nextWay.Nodes()
	if err != nil || nodes.Len() < 2 {
		utils.Logde(errors.Wrap(err, "could not read next way nodes"))
		return true
	}

	lastNode := nodes.At(nodes.Len() - 1)
	if lastNode.Latitude() == matchNode.Latitude() && lastNode.Longitude() == matchNode.Longitude() {
		return false
	}

	return true
}

func NextWay(way offline.Way, offlineMaps offline.Offline, isForward bool) (NextWayResult, error) {
	nodes, err := way.Nodes()
	if err != nil {
		return NextWayResult{}, errors.Wrap(err, "could not read way nodes")
	}
	if !way.HasNodes() || nodes.Len() == 0 {
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

	matchingWays, err := MatchingWays(way, offlineMaps, matchNode)
	if err != nil {
		return NextWayResult{StartPosition: matchNode}, errors.Wrap(err, "could not check for next ways")
	}

	if len(matchingWays) == 0 {
		return NextWayResult{StartPosition: matchNode}, nil
	}

	context := determineRoadContext(way)
	if context == CONTEXT_FREEWAY {
		filteredWays := []offline.Way{}
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
		candidates := []offline.Way{}
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
		candidates := []offline.Way{}
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
		candidates := []offline.Way{}
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

	validWays := []offline.Way{}
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

func isValidConnection(way offline.Way, matchNode, bearingNode offline.Coordinates, maxCurvature float64) bool {
	nodes, err := way.Nodes()
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

func selectBestCandidate(candidates []offline.Way, matchNode offline.Coordinates, context RoadContext) offline.Way {
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

func DistanceToEndOfWay(latitude float64, longitude float64, way offline.Way, isForward bool) (float64, error) {
	distanceResult, err := DistanceToWay(latitude, longitude, way)
	if err != nil {
		return 0, err
	}
	lat := distanceResult.LineEnd.Latitude()
	lon := distanceResult.LineEnd.Longitude()
	dist := DistanceToPoint(latitude*ms.TO_RADIANS, longitude*ms.TO_RADIANS, lat*ms.TO_RADIANS, lon*ms.TO_RADIANS)
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
		dist += DistanceToPoint(lat*ms.TO_RADIANS, lon*ms.TO_RADIANS, nLat*ms.TO_RADIANS, nLon*ms.TO_RADIANS)
		lat = nLat
		lon = nLon
	}
	return dist, nil
}

func NextWays(location log.GpsLocationData, currentWay CurrentWay, offlineMaps offline.Offline, isForward bool) ([]NextWayResult, error) {
	nextWays := []NextWayResult{}
	dist := 0.0
	wayIdx := currentWay.Way
	forward := isForward
	startLat := location.Latitude()
	startLon := location.Longitude()
	for dist < float64(MIN_WAY_DIST) {
		d, err := DistanceToEndOfWay(startLat, startLon, wayIdx, forward)
		if err != nil || d <= 0 {
			break
		}
		dist += d
		nw, err := NextWay(wayIdx, offlineMaps, forward)
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
		nextWay, err := NextWay(currentWay.Way, offlineMaps, isForward)
		if err != nil {
			return []NextWayResult{}, err
		}
		nextWays = append(nextWays, nextWay)
	}

	return nextWays, nil
}
