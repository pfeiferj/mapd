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
	m "pfeifer.dev/mapd/math"
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
	LineStart      m.Position
	LineEnd        m.Position
	LinePosition   m.LinePosition
	Distance       float32
}

// Updated CurrentWay struct with stability fields
type CurrentWay struct {
	Way               Way
	Distance          DistanceResult
	OnWay             OnWayResult
	StartPosition     m.Position
	EndPosition       m.Position
	ConfidenceCounter int
	LastChangeTime    time.Time
	StableDistance    float32
	SelectionType     custom.WaySelectionType
}

type NextWayResult struct {
	Way           Way
	IsForward     bool
	StartPosition m.Position
	EndPosition   m.Position
}

type Way struct {
	way offline.Way

	//calculated values
	width curry[float32, Way]
	context curry[RoadContext, Way]
	isFreeway curry[bool, Way]
	name curry[string, Way]
	distance curry[float32, Way]
	rank curry[int, Way]
	priority curry[int, Way]
	distanceMultiplier curry[float32, Way]

	//values from offline file
	oneWay curry[bool, Way]
	wayName curry[string, Way]
	wayRef curry[string, Way]
	maxSpeed curry[float64, Way]
	minPos curry[m.Position, Way]
	maxPos curry[m.Position, Way]
	nodes curry[[]m.Position, Way]
	lanes curry[int, Way]
	advisorySpeed curry[float64, Way]
	hazard curry[string, Way]
	maxSpeedForward curry[float64, Way]
	maxSpeedBackward curry[float64, Way]
}

func _nodes(w *Way) []m.Position {
	nodes, err := w.way.Nodes()
	if err != nil {
		return []m.Position{}
	}
	res := make([]m.Position, nodes.Len())
	for i := range nodes.Len() {
		node := nodes.At(i)
		res[i] = m.NewPosition(node.Latitude(), node.Longitude())
	}
	return res
}

func (w *Way) Nodes() []m.Position {
	return w.nodes.Value(w, _nodes)
}

func _oneWay(w *Way) bool {
	return w.way.OneWay()
}

func (w *Way) OneWay() bool {
	return w.oneWay.Value(w, _oneWay)
}

func _wayName(w *Way) string {
	wn, err := w.way.Name()
	if err != nil {
		wn = ""
	}
	return wn
}

func (w *Way) WayName() string {
	return w.wayName.Value(w, _wayName)
}

func _wayRef(w *Way) string {
	wr, err := w.way.Ref()
	if err != nil {
		wr = ""
	}
	return wr
}

func (w *Way) WayRef() string {
	return w.wayRef.Value(w, _wayRef)
}

func _maxSpeed(w *Way) float64 {
	return w.way.MaxSpeed()
}

func (w *Way) MaxSpeed() float64 {
	return w.maxSpeed.Value(w, _maxSpeed)
}

func _maxSpeedForward(w *Way) float64 {
	return w.way.MaxSpeedForward()
}

func (w *Way) MaxSpeedForward() float64 {
	return w.maxSpeedForward.Value(w, _maxSpeedForward)
}

func _maxSpeedBackward(w *Way) float64 {
	return w.way.MaxSpeedBackward()
}

func (w *Way) MaxSpeedBackward() float64 {
	return w.maxSpeedBackward.Value(w, _maxSpeedBackward)
}

func _minPos(w *Way) m.Position {
	return m.NewPosition(w.way.MinLat(), w.way.MinLon())
}

func (w *Way) MinPos() m.Position {
	return w.minPos.Value(w, _minPos)
}

func _maxPos(w *Way) m.Position {
	return m.NewPosition(w.way.MinLat(), w.way.MinLon())
}

func (w *Way) MaxPos() m.Position {
	return w.maxPos.Value(w, _maxPos)
}

func _lanes(w *Way) int {
	return int(w.way.Lanes())
}

func (w *Way) Lanes() int {
	return w.lanes.Value(w, _lanes)
}

func _advisorySpeed(w *Way) float64 {
	return w.way.AdvisorySpeed()
}

func (w *Way) AdvisorySpeed() float64 {
	return w.advisorySpeed.Value(w, _advisorySpeed)
}

func _hazard(w *Way) string {
	hazard, err := w.way.Hazard()
	if err != nil {
		hazard = ""
	}
	return hazard
}

func (w *Way) Hazard() string {
	return w.hazard.Value(w, _hazard)
}

func (w *Way) OnWay(location log.GpsLocationData, distanceMultiplier float32) (OnWayResult, error) {
	res := OnWayResult{}
	pos := m.NewPosition(location.Latitude(), location.Longitude())
	d, err := w.DistanceFrom(pos)
	res.Distance = d
	if err != nil {
		res.OnWay = false
		return res, errors.Wrap(err, "could not get distance to way")
	}
	max_dist := max(location.HorizontalAccuracy(), 5) + w.Width()
	max_dist *= distanceMultiplier

	if d.Distance < max_dist {
		res.OnWay = true
		res.IsForward = IsForward(d.LineStart, d.LineEnd, float64(location.BearingDeg()))
		if !res.IsForward && w.OneWay() {
			res.OnWay = false
		}
		return res, nil
	}
	res.OnWay = false
	return res, nil
}


func (w *Way) bearingAlignment(location log.GpsLocationData) (float32, error) {
	pos := m.NewPosition(location.Latitude(), location.Longitude())
	d, err := w.DistanceFrom(pos)
	if err != nil {
		return 1.0, err
	}

	lineVec := d.LineStart.VectorTo(d.LineEnd)
	wayBearing := lineVec.Bearing()

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

		score += float32(100 - way.Rank())

		bearingAlignment, err := way.bearingAlignment(location)
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

func (w *Way) DistanceFrom(pos m.Position) (DistanceResult, error) {
	res := DistanceResult{}
	var minNodeStart m.Position
	var minNodeEnd m.Position
	minDistance := float32(math.MaxFloat32)
	nodes := w.Nodes()
	if len(nodes) < 2 {
		return res, errors.New("not enough nodes to determine distance")
	}

	minLinePosition := m.LinePosition{}
	minIdx := 0
	for i := 0; i < len(nodes)-1; i++ {
		nodeStart := nodes[i]
		nodeEnd := nodes[i + 1]
		line := m.Line{Start: nodeStart, End: nodeEnd}
		linePosition := line.NearestPosition(pos)
		distance := pos.DistanceTo(linePosition.Pos)

		if distance < minDistance {
			minDistance = distance
			minNodeStart = nodeStart
			minNodeEnd = nodeEnd
			minLinePosition = linePosition
			minIdx = i
		}
	}
	onWayDistance := minNodeStart.DistanceTo(minLinePosition.Pos)
	for i := range minIdx {
		nodeStart := nodes[i]
		nodeEnd := nodes[i + 1]
		onWayDistance += nodeStart.DistanceTo(nodeEnd)
	}

	res.Distance = minDistance
	res.LineStart = minNodeStart
	res.LineEnd = minNodeEnd
	res.LinePosition = minLinePosition
	return res, nil
}

func (w *Way) GetStartEnd(isForward bool) (m.Position, m.Position) {
	nodes := w.Nodes()
	if len(nodes) == 0 {
		return m.Position{}, m.Position{}
	}

	if len(nodes) == 1 {
		return nodes[0], nodes[0]
	}

	if isForward {
		return nodes[0], nodes[len(nodes) - 1]
	}
	return nodes[len(nodes) - 1], nodes[0]
}

func GetCurrentWay(currentWay CurrentWay, nextWays []NextWayResult, offline offline.Offline, location log.GpsLocationData) (CurrentWay, error) {
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

func getPossibleWays(offlineMaps offline.Offline, location log.GpsLocationData) ([]Way, error) {
	possibleWays := []Way{}
	ways, err := offlineMaps.Ways()
	if err != nil {
		return possibleWays, errors.Wrap(err, "could not get other ways")
	}

	for i := 0; i < ways.Len(); i++ {
		way := ways.At(i)
		w := Way{way: way}
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

func IsForward(lineStart m.Position, lineEnd m.Position, bearing float64) bool {

	vec := lineStart.VectorTo(lineEnd)
	wayBearing := vec.Bearing()
	bearingDelta := math.Abs(bearing*ms.TO_RADIANS - wayBearing)
	return math.Cos(bearingDelta) >= 0
}

func (w *Way) MatchingWays(offlineMaps offline.Offline, matchNode m.Position) ([]Way, error) {
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

		minPos := w.MinPos()
		maxPos := w.MaxPos()
		if way.MinLat() == minPos.Lat() && way.MaxLat() == maxPos.Lat() && way.MinLon() == minPos.Lon() && way.MaxLon() == maxPos.Lon() {
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
		if (fNode.Latitude() == matchNode.Lat() && fNode.Longitude() == matchNode.Lon()) || (lNode.Latitude() == matchNode.Lat() && lNode.Longitude() == matchNode.Lon()) {
			w := Way{way: way}
			matchingWays = append(matchingWays, w)
		}
	}

	return matchingWays, nil
}

func NextIsForward(nextWay Way, matchNode m.Position) bool {
	if len(nextWay.Nodes()) == 0 {
		return true
	}
	nodes := nextWay.Nodes()
	if len(nodes) < 2 {
		return true
	}

	lastNode := nodes[len(nodes) - 1]
	if lastNode.Equals(matchNode) {
		return false
	}

	return true
}

func (w *Way) NextWay(offlineMaps offline.Offline, isForward bool) (NextWayResult, error) {
	nodes := w.Nodes()
	if len(nodes) == 0 {
		return NextWayResult{}, nil
	}

	var matchNode m.Position
	var matchBearingNode m.Position
	if isForward {
		matchNode = nodes[len(nodes) - 1]
		if len(nodes) > 1 {
			matchBearingNode = nodes[len(nodes) - 2]
		}
	} else {
		matchNode = nodes[0]
		if len(nodes) > 1 {
			matchBearingNode = nodes[1]
		}
	}

	if !maps.PointInBox(matchNode.Lat(), matchNode.Lon(), offlineMaps.MinLat()-offlineMaps.Overlap(), offlineMaps.MinLon()-offlineMaps.Overlap(), offlineMaps.MaxLat()+offlineMaps.Overlap(), offlineMaps.MaxLon()+offlineMaps.Overlap()) {
		return NextWayResult{}, nil
	}

	matchingWays, err := w.MatchingWays(offlineMaps, matchNode)
	if err != nil {
		return NextWayResult{StartPosition: matchNode}, errors.Wrap(err, "could not check for next ways")
	}

	if len(matchingWays) == 0 {
		return NextWayResult{StartPosition: matchNode}, nil
	}

	if w.Context() == CONTEXT_FREEWAY {
		filteredWays := []Way{}
		for _, mWay := range matchingWays {
			name := mWay.WayName()
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
	if w.Context() == CONTEXT_CITY {
		curvatureThreshold = 0.3
	} else if w.Context() == CONTEXT_FREEWAY {
		curvatureThreshold = 0.1
	}

	name := w.WayName()
	if len(name) > 0 {
		candidates := []Way{}
		for _, mWay := range matchingWays {
			mName := mWay.WayName()
			if mName == name {
				isForward := NextIsForward(mWay, matchNode)
				if !isForward && mWay.OneWay() {
					continue
				}

				if len(nodes) > 1 && mWay.isValidConnection(matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, w.Context())
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

	ref := w.WayRef()
	if len(ref) > 0 {
		candidates := []Way{}
		for _, mWay := range matchingWays {
			mRef := mWay.WayRef()
			if mRef == ref {
				isForward := NextIsForward(mWay, matchNode)
				if !isForward && mWay.OneWay() {
					continue
				}

				if len(nodes) > 1 && mWay.isValidConnection(matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, w.Context())
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
			mRef := mWay.WayRef()
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

				if len(nodes) > 1 && mWay.isValidConnection(matchNode, matchBearingNode, curvatureThreshold) {
					candidates = append(candidates, mWay)
				}
			}
		}

		if len(candidates) > 0 {
			bestWay := selectBestCandidate(candidates, matchNode, w.Context())
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
		if !isForward && mWay.OneWay() {
			continue
		}
		if len(nodes) > 1 && mWay.isValidConnection(matchNode, matchBearingNode, curvatureThreshold) {
			validWays = append(validWays, mWay)
		}
	}

	if len(validWays) > 0 {
		bestWay := selectBestCandidate(validWays, matchNode, w.Context())
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

func (w *Way) isValidConnection(matchNode, bearingNode m.Position, maxCurvature float64) bool {
	nodes := w.Nodes()
	if len(nodes) < 2 {
		return false
	}

	var nextBearingNode m.Position
	if matchNode.Equals(nodes[0]) {
		nextBearingNode = nodes[1]
	} else {
		nextBearingNode = nodes[len(nodes) - 2]
	}

	curv := m.CalculateCurvature(bearingNode, matchNode, nextBearingNode)
	return math.Abs(curv.Curvature) <= maxCurvature
}

func selectBestCandidate(candidates []Way, matchNode m.Position, context RoadContext) Way {
	if len(candidates) == 1 {
		return candidates[0]
	}

	bestWay := candidates[0]
	bestScore := float64(-1000)

	for _, way := range candidates {
		score := float64(way.Priority())

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

func (w *Way) DistanceToEnd(pos m.Position, isForward bool) (float32, error) {
	distanceResult, err := w.DistanceFrom(pos)
	if err != nil {
		return 0, err
	}
	dist := pos.DistanceTo(distanceResult.LineEnd)
	stopFiltering := false
	nodes := w.Nodes()
	if err != nil {
		return 0, err
	}
	lastPos := distanceResult.LineEnd
	for i := 0; i < len(nodes); i++ {
		index := i
		if !isForward {
			index = len(nodes) - 1 - i
		}
		node := nodes[index]
		if node.Equals(lastPos) && !stopFiltering {
			stopFiltering = true
		}
		if !stopFiltering {
			continue
		}
		
		dist += lastPos.DistanceTo(node)
		lastPos = node
	}
	return dist, nil
}

func NextWays(location log.GpsLocationData, currentWay CurrentWay, offlineMaps offline.Offline, isForward bool) ([]NextWayResult, error) {
	nextWays := []NextWayResult{}
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
			return []NextWayResult{}, err
		}
		nextWays = append(nextWays, nextWay)
	}

	return nextWays, nil
}

func _distance(w *Way) float32 {
	nodes := w.Nodes()

	if len(nodes) < 2 {
		return 0
	}

	totalDistance := float32(0.0)
	for i := range len(nodes) - 1 {
		nodeStart := nodes[i]
		nodeEnd := nodes[i + 1]
		distance := nodeStart.DistanceTo(nodeEnd)
		totalDistance += distance
	}

	return totalDistance
}

func (w *Way) Distance() (float32) {
	return w.distance.Value(w, _distance)
}

func _name(w *Way) string {
	name, err := w.way.Name()
	if err == nil {
		if len(name) > 0 {
			return name
		}
	}
	ref, err := w.way.Ref()
	if err == nil {
		if len(ref) > 0 {
			return ref
		}
	}
	return ""
}

func (w *Way) Name() string {
	return w.name.Value(w, _name)
}

func _width(w *Way) float32 {
	lanes := w.way.Lanes()
	if lanes == 0 {
		lanes = 2
	}
	width := float32(lanes) * ms.Settings.DefaultLaneWidth
	return width
}

func (w *Way) Width() float32 {
	return w.width.Value(w, _width)
}

func _context(w *Way) RoadContext {
	lanes := w.way.Lanes()
	name, _ := w.way.Name()
	ref, _ := w.way.Ref()

	if w.IsFreeway() || lanes >= 4 {
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

func (w *Way) Context() RoadContext {
	return w.context.Value(w, _context)
}

func _isFreeway(w *Way) bool {
	lanes := w.way.Lanes()
	name, _ := w.way.Name()
	ref, _ := w.way.Ref()

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

func (w *Way) IsFreeway() bool {
	return w.isFreeway.Value(w, _isFreeway)
}

func _rank(w *Way) int {
	name, _ := w.way.Name()
	ref, _ := w.way.Ref()
	lanes := w.way.Lanes()

	// Infer highway type from characteristics
	if w.IsFreeway() {
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

// Get highway hierarchy rank for a way
func (w *Way) Rank() int {
	return w.rank.Value(w, _rank)
}

func _priority(w *Way) int {
	lanes := w.way.Lanes()
	name, _ := w.way.Name()
	ref, _ := w.way.Ref()

	priority := LANE_COUNT_PRIORITY[lanes]
	if priority == 0 {
		priority = 30
	}

	switch w.Context() {
	case CONTEXT_FREEWAY:
		if w.IsFreeway() {
			priority += 30
		}
		nameUpper := strings.ToUpper(name)
		if strings.Contains(nameUpper, "STREET") ||
			strings.Contains(nameUpper, "AVENUE") {
			priority -= 40
		}

	case CONTEXT_CITY:
		if w.IsFreeway() {
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
		if w.IsFreeway() {
			priority += 20
		}
	}

	if len(ref) > 0 {
		priority += 10
	}

	return priority
}

func (w *Way) Priority() int {
	return w.priority.Value(w, _priority)
}

func _distanceMultiplier(w *Way) float32 {
	switch w.Context() {
	case CONTEXT_CITY:
		return 0.75
	case CONTEXT_FREEWAY:
		return 1.5
	default:
		return 1
	}
}

func (w *Way) DistanceMultiplier() float32 {
	return w.distanceMultiplier.Value(w, _distanceMultiplier)
}
