package main

import (
	"fmt"
	"math"

	"github.com/pkg/errors"
	ms "pfeifer.dev/mapd/settings"
)

func Dot(ax float64, ay float64, bx float64, by float64) (product float64) {
	return (ax * bx) + (ay * by)
}

// Point represents a 2D point
type Point struct {
	X, Y float64
}

type LinePoint struct {
	X, Y, T float64
}

// Subtract returns the vector from other to p
func (p Point) Subtract(other Point) Point {
	return Point{X: p.X - other.X, Y: p.Y - other.Y}
}

// Add returns the sum of two points/vectors
func (p Point) Add(other Point) Point {
	return Point{X: p.X + other.X, Y: p.Y + other.Y}
}

// Scale returns the point/vector scaled by a factor
func (p Point) Scale(factor float64) Point {
	return Point{X: p.X * factor, Y: p.Y * factor}
}

// Dot returns the dot product of two vectors
func (p Point) Dot(other Point) float64 {
	return p.X*other.X + p.Y*other.Y
}

func PointOnLine(startLat float64, startLon float64, endLat float64, endLon float64, lat float64, lon float64) (LinePoint) {
	A := Point {
		X: startLat,
		Y: startLon,
	}
	B := Point {
		X: endLat,
		Y: endLon,
	}
	P := Point {
		X: lat,
		Y: lon,
	}

	AB := B.Subtract(A)
	AP := P.Subtract(A)

	// Project P onto AB, get parameter t
	t := AP.Dot(AB) / AB.Dot(AB)

	// Clamp to segment [0, 1]
	t = math.Max(0, math.Min(1, t))

	// Calculate closest point
	closest := A.Add(AB.Scale(t))

	res := LinePoint{X: closest.X, Y: closest.Y, T: t}
	return res
}

// arguments should be in radians
func DistanceToPoint(ax float64, ay float64, bx float64, by float64) (meters float32) {
	a := math.Sin((bx-ax)/2)*math.Sin((bx-ax)/2) + math.Cos(ax)*math.Cos(bx)*math.Sin((by-ay)/2)*math.Sin((by-ay)/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return float32(ms.R * c) // in metres
}

func Vector(latA float64, lonA float64, latB float64, lonB float64) (x float64, y float64) {
	dlon := lonB - lonA
	x = math.Sin(dlon) * math.Cos(latB)
	y = math.Cos(latA)*math.Sin(latB) - (math.Sin(latA) * math.Cos(latB) * math.Cos(dlon))
	return x, y
}

func Bearing(latA float64, lonA float64, latB float64, lonB float64) (radians float64) {
	latA = latA * ms.TO_RADIANS
	latB = latB * ms.TO_RADIANS
	lonA = lonA * ms.TO_RADIANS
	lonB = lonB * ms.TO_RADIANS
	x, y := Vector(latA, lonA, latB, lonB)
	return math.Atan2(x, y)
}

type Curvature struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Curvature float64 `json:"curvature"`
}

func GetStateCurvatures(state *State) ([]Curvature, error) {
	nodes := state.CurrentWay.Way.Nodes()
	num_points := len(nodes)
	all_nodes := [][]Position{nodes}
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

	x_points := make([]float64, num_points)
	y_points := make([]float64, num_points)

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
		x_points[i] = node.Lat()
		y_points[i] = node.Lon()

		nodes_idx += 1
		if nodes_idx == len(all_nodes[all_nodes_idx]) || (nodes_idx == len(all_nodes[all_nodes_idx])-1 && all_nodes_idx > 0) {
			all_nodes_idx += 1
			nodes_idx = 0
			if all_nodes_idx < len(all_nodes_is_merge_or_split) && all_nodes_is_merge_or_split[all_nodes_idx] {
				merge_or_split_nodes = append(merge_or_split_nodes, i)
			}
		}
	}

	curvatures, arc_lengths, err := GetCurvatures(x_points, y_points)
	if err != nil {
		return []Curvature{}, errors.Wrap(err, "could not get curvatures from points")
	}

	// set the merge nodes to be straight to help balance out issues with map representation
	for _, merge_or_split_node := range merge_or_split_nodes {
		if merge_or_split_node >= 2 {
			curvatures[merge_or_split_node-2] = 0.0015
			curvatures[merge_or_split_node-1] = 0.0015
		}
		// also include nodes within 15 meters
		for i := merge_or_split_node - 3; i >= 0; i-- {
			if DistanceToPoint(x_points[merge_or_split_node]*ms.TO_RADIANS, y_points[merge_or_split_node]*ms.TO_RADIANS, x_points[i]*ms.TO_RADIANS, y_points[i]*ms.TO_RADIANS) > 15 {
				break
			}
			curvatures[i] = 0.0015
		}
		// also include forward nodes within 15 meters
		for i := merge_or_split_node; i < len(curvatures); i++ {
			if DistanceToPoint(x_points[merge_or_split_node]*ms.TO_RADIANS, y_points[merge_or_split_node]*ms.TO_RADIANS, x_points[i]*ms.TO_RADIANS, y_points[i]*ms.TO_RADIANS) > 15 {
				break
			}
			curvatures[i] = 0.0015
		}
	}

	average_curvatures, err := GetAverageCurvatures(curvatures, arc_lengths)
	if err != nil {
		return []Curvature{}, errors.Wrap(err, "could not get average curvatures from curvatures")
	}
	curvature_outputs := make([]Curvature, len(average_curvatures))
	for i, curvature := range average_curvatures {
		curvature_outputs[i].Curvature = curvature
		curvature_outputs[i].Latitude = x_points[i+2]
		curvature_outputs[i].Longitude = y_points[i+2]
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

func GetAverageCurvatures(curvatures []float64, arc_lengths []float64) (average_curvatures []float64, err error) {
	if len(curvatures) < 3 {
		return []float64{}, errors.New("not enough curvatures to average")
	}

	average_curvatures = make([]float64, len(curvatures)-2)

	for i := 0; i < len(curvatures)-2; i++ {
		a := curvatures[i]
		b := curvatures[i+1]
		c := curvatures[i+2]
		al := arc_lengths[i]
		bl := arc_lengths[i+1]
		cl := arc_lengths[i+2]

		if al+bl+cl == 0 {
			average_curvatures[i] = 0
			continue
		}

		average_curvatures[i] = (a*al + b*bl + c*cl) / (al + bl + cl)
	}

	return average_curvatures, nil
}

func GetCurvatures(x_points []float64, y_points []float64) (curvatures []float64, arc_lengths []float64, err error) {
	if len(x_points) < 3 {
		return []float64{}, []float64{}, errors.New(fmt.Sprintf("not enough points to calculate curvatures. len(points): %d", len(x_points)))
	}
	curvatures = make([]float64, len(x_points)-2)
	arc_lengths = make([]float64, len(x_points)-2)

	for i := 0; i < len(x_points)-2; i++ {
		curvature, arc_length, _ := GetCurvature(x_points[i], y_points[i], x_points[i+1], y_points[i+1], x_points[i+2], y_points[i+2])

		curvatures[i] = curvature

		arc_lengths[i] = arc_length
	}
	return curvatures, arc_lengths, nil
}

func GetCurvature(x_a float64, y_a float64, x_b float64, y_b float64, x_c float64, y_c float64) (curvature float64, arc_length float64, angle float64) {
	length_a := DistanceToPoint(x_a*ms.TO_RADIANS, y_a*ms.TO_RADIANS, x_b*ms.TO_RADIANS, y_b*ms.TO_RADIANS)
	length_b := DistanceToPoint(x_a*ms.TO_RADIANS, y_a*ms.TO_RADIANS, x_c*ms.TO_RADIANS, y_c*ms.TO_RADIANS)
	length_c := DistanceToPoint(x_b*ms.TO_RADIANS, y_b*ms.TO_RADIANS, x_c*ms.TO_RADIANS, y_c*ms.TO_RADIANS)

	sp := (length_a + length_b + length_c) / 2

	area := float32(math.Sqrt(float64(sp * (sp - length_a) * (sp - length_b) * (sp - length_c))))

	if length_a*length_b*length_c == 0 {
		return 0, 0, 0
	}

	curvature = float64((4 * area) / (length_a * length_b * length_c))

	radius := 1.0 / curvature

	angle = math.Acos((math.Pow(radius, 2)*2 - math.Pow(float64(length_b), 2)) / (2 * math.Pow(radius, 2)))
	arc_length = radius * angle
	return curvature, arc_length, angle
}
