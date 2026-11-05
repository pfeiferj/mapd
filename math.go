package main

import (
	"fmt"
	"math"
	"slices"

	"github.com/pkg/errors"
	"pfeifer.dev/mapd/maps"
	m "pfeifer.dev/mapd/math"
	ms "pfeifer.dev/mapd/settings"
)

// the current way's nodes followed by each next way's, in the direction of
// travel, with each junction node listed once
func GetStatePath(state *State) []m.Position {
	positions := wayNodesInTravelOrder(&state.CurrentWay.Way, state.CurrentWay.OnWay.IsForward)
	for _, nextWay := range state.NextWays {
		nodes := wayNodesInTravelOrder(&nextWay.Way, nextWay.IsForward)
		if len(nodes) > 0 {
			positions = append(positions, nodes[1:]...)
		}
	}
	return positions
}

func wayNodesInTravelOrder(way *maps.Way, isForward bool) []m.Position {
	nodes := way.Nodes.Slice()
	if !isForward {
		slices.Reverse(nodes)
	}
	return nodes
}

func GetStateCurvatures(state *State) ([]m.Curvature, error) {
	positions := GetStatePath(state)

	merge_or_split_nodes := []int{}
	junction_idx := state.CurrentWay.Way.Nodes.Len() - 1
	lastWay := state.CurrentWay.Way
	for _, nextWay := range state.NextWays {
		if lastWay.Lanes() < nextWay.Way.Lanes() || (lastWay.Lanes() > nextWay.Way.Lanes() && !lastWay.OneWay() && nextWay.Way.OneWay()) {
			merge_or_split_nodes = append(merge_or_split_nodes, junction_idx)
		}
		junction_idx += nextWay.Way.Nodes.Len() - 1
		lastWay = nextWay.Way
	}

	curvatures, err := GetCurvatures(positions)
	if err != nil {
		return []m.Curvature{}, errors.Wrap(err, "could not get curvatures from points")
	}

	// set the merge nodes to be straight to help balance out issues with map representation
	for _, merge_or_split_node := range merge_or_split_nodes {
		if merge_or_split_node >= 2 {
			curvatures[merge_or_split_node-2].Curvature = 0.0015
			curvatures[merge_or_split_node-1].Curvature = 0.0015
		}
		// also include nodes within 15 meters
		for i := merge_or_split_node - 3; i >= 0; i-- {

			if positions[merge_or_split_node].DistanceTo(positions[i]) > 15 {
				break
			}
			curvatures[i].Curvature = 0.0015
		}
		// also include forward nodes within 15 meters
		for i := merge_or_split_node; i < len(curvatures); i++ {
			if positions[merge_or_split_node].DistanceTo(positions[i]) > 15 {
				break
			}
			curvatures[i].Curvature = 0.0015
		}
	}

	average_curvatures, err := GetAverageCurvatures(curvatures)
	if err != nil {
		return []m.Curvature{}, errors.Wrap(err, "could not get average curvatures from curvatures")
	}
	return average_curvatures, nil
}

type Velocity struct {
	Pos             m.Position
	Velocity        float64
	TriggerDistance float32
}

func GetTargetVelocities(curvatures []m.Curvature, previousTargets []Velocity) (velocities []Velocity) {
	velocities = make([]Velocity, len(curvatures))
	for i, curv := range curvatures {
		if curv.Curvature == 0 {
			continue
		}
		velocities[i].Velocity = math.Pow(float64(ms.Settings.CurrentPersonality().MapCurveTargetLatA)/curv.Curvature, 1.0/2)
		velocities[i].Pos = curv.Pos
		for _, t := range previousTargets {
			if velocities[i].Pos.Equals(t.Pos) {
				velocities[i].TriggerDistance = t.TriggerDistance
			}
		}
	}
	return velocities
}

func GetAverageCurvatures(curvatures []m.Curvature) (average_curvatures []m.Curvature, err error) {
	if len(curvatures) < 3 {
		return []m.Curvature{}, errors.New("not enough curvatures to average")
	}

	average_curvatures = make([]m.Curvature, len(curvatures)-2)

	for i := 0; i < len(curvatures)-2; i++ {
		a := curvatures[i].Curvature
		b := curvatures[i+1].Curvature
		c := curvatures[i+2].Curvature
		al := curvatures[i].ArcLength
		bl := curvatures[i+1].ArcLength
		cl := curvatures[i+2].ArcLength

		if al+bl+cl == 0 {
			average_curvatures[i] = curvatures[i+2]
			continue
		}

		avg := m.Curvature{Pos: curvatures[i+1].Pos}
		avg.Curvature = (a*al + b*bl + c*cl) / (al + bl + cl)
		avg.ArcLength = (curvatures[i].ArcLength + curvatures[i+1].ArcLength + curvatures[i+2].ArcLength) / 3
		avg.Angle = (curvatures[i].Angle + curvatures[i+1].Angle + curvatures[i+2].Angle) / 3
		average_curvatures[i] = avg
	}

	return average_curvatures, nil
}

func GetCurvatures(positions []m.Position) (curvatures []m.Curvature, err error) {
	if len(positions) < 3 {
		return []m.Curvature{}, errors.New(fmt.Sprintf("not enough points to calculate curvatures. len(points): %d", len(positions)))
	}
	curvatures = make([]m.Curvature, len(positions)-2)

	for i := 0; i < len(positions)-2; i++ {
		curvature := m.CalculateCurvature(positions[i], positions[i+1], positions[i+2])

		curvatures[i] = curvature
	}

	return curvatures, nil
}
