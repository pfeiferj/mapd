package main

import (
	"fmt"
	"math"

	"github.com/pkg/errors"
	ms "pfeifer.dev/mapd/settings"
	m "pfeifer.dev/mapd/math"
)

type Curvature struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Curvature float64 `json:"curvature"`
}

func GetStateCurvatures(state *State) ([]Curvature, error) {
	nodes := state.CurrentWay.Way.Nodes()
	num_points := len(nodes)
	all_nodes := [][]m.Position{nodes}
	all_nodes_direction := []bool{state.CurrentWay.OnWay.IsForward}
	all_nodes_is_merge_or_split := []bool{false}
	lastWay := state.CurrentWay.Way
	for _, nextWay := range state.NextWays {
		nwNodes := nextWay.Way.Nodes()
		if len(nwNodes) > 0 {
			num_points += len(nwNodes) - 1
		}
		all_nodes = append(all_nodes, nwNodes)
		all_nodes_direction = append(all_nodes_direction, nextWay.IsForward)
		all_nodes_is_merge_or_split = append(all_nodes_is_merge_or_split, lastWay.Lanes() < nextWay.Way.Lanes() || (lastWay.Lanes() > nextWay.Way.Lanes() && !lastWay.OneWay() && nextWay.Way.OneWay()))
		lastWay = nextWay.Way
	}

	positions := make([]m.Position, num_points)

	merge_or_split_nodes := []int{}
	all_nodes_idx := 0
	nodes_idx := 0
	for i := 0; i < num_points; i++ {
		var index int
		forward := all_nodes_direction[all_nodes_idx]
		if forward {
			index = nodes_idx
			if all_nodes_idx > 0 {
				index += 1
			}
		} else {
			index = len(all_nodes[all_nodes_idx]) - nodes_idx - 1
			if all_nodes_idx > 0 {
				index -= 1
			}
		}
		node := all_nodes[all_nodes_idx][index]
		positions[i] = node

		nodes_idx += 1
		if nodes_idx == len(all_nodes[all_nodes_idx]) || (nodes_idx == len(all_nodes[all_nodes_idx])-1 && all_nodes_idx > 0) {
			all_nodes_idx += 1
			nodes_idx = 0
			if all_nodes_idx < len(all_nodes_is_merge_or_split) && all_nodes_is_merge_or_split[all_nodes_idx] {
				merge_or_split_nodes = append(merge_or_split_nodes, i)
			}
		}
	}

	curvatures, err := GetCurvatures(positions)
	if err != nil {
		return []Curvature{}, errors.Wrap(err, "could not get curvatures from points")
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
		return []Curvature{}, errors.Wrap(err, "could not get average curvatures from curvatures")
	}
	curvature_outputs := make([]Curvature, len(average_curvatures))
	for i, curvature := range average_curvatures {
		curvature_outputs[i].Curvature = curvature
		curvature_outputs[i].Latitude = positions[i+2].Lat()
		curvature_outputs[i].Longitude = positions[i+2].Lon()
	}
	return curvature_outputs, nil
}

type Velocity struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Velocity  float64 `json:"velocity"`
}

func GetTargetVelocities(curvatures []Curvature) (velocities []Velocity) {
	velocities = make([]Velocity, len(curvatures))
	for i, curv := range curvatures {
		if curv.Curvature == 0 {
			continue
		}
		velocities[i].Velocity = math.Pow(float64(ms.Settings.CurveTargetLatA)/curv.Curvature, 1.0/2)
		velocities[i].Latitude = curv.Latitude
		velocities[i].Longitude = curv.Longitude
	}
	return velocities
}

func GetAverageCurvatures(curvatures []m.Curvature) (average_curvatures []float64, err error) {
	if len(curvatures) < 3 {
		return []float64{}, errors.New("not enough curvatures to average")
	}

	average_curvatures = make([]float64, len(curvatures)-2)

	for i := 0; i < len(curvatures)-2; i++ {
		a := curvatures[i].Curvature
		b := curvatures[i+1].Curvature
		c := curvatures[i+2].Curvature
		al := curvatures[i].ArcLength
		bl := curvatures[i+1].ArcLength
		cl := curvatures[i+2].ArcLength

		if al+bl+cl == 0 {
			average_curvatures[i] = 0
			continue
		}

		average_curvatures[i] = (a*al + b*bl + c*cl) / (al + bl + cl)
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

