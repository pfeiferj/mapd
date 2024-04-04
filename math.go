package main

import (
	"math"

	"capnproto.org/go/capnp/v3"
	"github.com/pkg/errors"
)

var (
	R                = 6373000.0           // approximate radius of earth in meters
	LANE_WIDTH       = 3.7                 // meters
	QUERY_RADIUS     = float64(3000)       // meters
	PADDING          = 10 / R * TO_DEGREES // 10 meters in degrees
	TO_RADIANS       = math.Pi / 180
	TO_DEGREES       = 180 / math.Pi
	TARGET_LAT_ACCEL = 2.0 // m/s^2
)

func Dot(ax float64, ay float64, bx float64, by float64) float64 {
	return (ax * bx) + (ay * by)
}

func PointOnLine(startLat float64, startLon float64, endLat float64, endLon float64, lat float64, lon float64) (float64, float64) {
	aplat := lat - startLat
	aplon := lon - startLon

	ablat := endLat - startLat
	ablon := endLon - startLon

	t := Dot(aplat, aplon, ablat, ablon) / Dot(ablat, ablon, ablat, ablon)

	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	latitude := startLat + t*ablat
	longitude := startLon + t*ablon

	return latitude, longitude
}

// arguments should be in radians
func DistanceToPoint(ax float64, ay float64, bx float64, by float64) float64 {
	a := math.Sin((bx-ax)/2)*math.Sin((bx-ax)/2) + math.Cos(ax)*math.Cos(bx)*math.Sin((by-ay)/2)*math.Sin((by-ay)/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c // in metres
}

func Vector(latA float64, lonA float64, latB float64, lonB float64) (float64, float64) {
	dlon := lonB - lonA
	x := math.Sin(dlon) * math.Cos(latB)
	y := math.Cos(latA)*math.Sin(latB) - (math.Sin(latA) * math.Cos(latB) * math.Cos(dlon))
	return x, y
}

func Bearing(latA float64, lonA float64, latB float64, lonB float64) float64 {
	latA = latA * TO_RADIANS
	latB = latB * TO_RADIANS
	lonA = lonA * TO_RADIANS
	lonB = lonB * TO_RADIANS
	x, y := Vector(latA, lonA, latB, lonB)
	return math.Atan2(x, y)
}

type Curvature struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Curvature float64 `json:"curvature"`
}

func GetStateCurvatures(state *State) ([]Curvature, error) {
	nodes, err := state.CurrentWay.Way.Nodes()
	if err != nil {
		return []Curvature{}, errors.Wrap(err, "could not read way nodes")
	}
	num_points := nodes.Len()
	all_nodes := []capnp.StructList[Coordinates]{nodes}
	all_nodes_direction := []bool{state.CurrentWay.OnWay.IsForward}
	for _, nextWay := range state.NextWays {
		nwNodes, err := nextWay.Way.Nodes()
		if err != nil {
			continue
		}
		if nwNodes.Len() > 0 {
			num_points += nwNodes.Len() - 1
		}
		all_nodes = append(all_nodes, nwNodes)
		all_nodes_direction = append(all_nodes_direction, nextWay.IsForward)
	}

	x_points := make([]float64, num_points)
	y_points := make([]float64, num_points)

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
			index = all_nodes[all_nodes_idx].Len() - nodes_idx - 1
			if all_nodes_idx > 0 {
				index -= 1
			}
		}
		node := all_nodes[all_nodes_idx].At(index)
		x_points[i] = node.Latitude()
		y_points[i] = node.Longitude()

		nodes_idx += 1
		if nodes_idx == all_nodes[all_nodes_idx].Len() || (nodes_idx == all_nodes[all_nodes_idx].Len()-1 && all_nodes_idx > 0) {
			all_nodes_idx += 1
			nodes_idx = 0
		}
	}

	curvatures, arc_lengths, err := GetCurvatures(x_points, y_points)
	if err != nil {
		return []Curvature{}, errors.Wrap(err, "could not get curvatures from points")
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

func GetTargetVelocities(curvatures []Curvature) []Velocity {
	velocities := make([]Velocity, len(curvatures))
	for i, curv := range curvatures {
		if curv.Curvature == 0 {
			continue
		}
		velocities[i].Velocity = math.Pow(TARGET_LAT_ACCEL/curv.Curvature, 1.0/2)
		velocities[i].Latitude = curv.Latitude
		velocities[i].Longitude = curv.Longitude
	}
	return velocities
}

func GetAverageCurvatures(curvatures []float64, arc_lengths []float64) ([]float64, error) {
	if len(curvatures) < 3 {
		return []float64{}, errors.New("not enough curvatures to average")
	}

	average_curvatures := make([]float64, len(curvatures)-2)

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

func GetCurvatures(x_points []float64, y_points []float64) ([]float64, []float64, error) {
	if len(x_points) < 3 {
		return []float64{}, []float64{}, errors.New("not enough points to calculate curvatures")
	}
	curvatures := make([]float64, len(x_points)-2)
	arc_lengths := make([]float64, len(x_points)-2)

	for i := 0; i < len(x_points)-2; i++ {
		curvature, arc_length, _ := GetCurvature(x_points[i], y_points[i], x_points[i+1], y_points[i+1], x_points[i+2], y_points[i+2])

		curvatures[i] = curvature

		arc_lengths[i] = arc_length
	}
	return curvatures, arc_lengths, nil
}

func GetCurvature(x_a float64, y_a float64, x_b float64, y_b float64, x_c float64, y_c float64) (float64, float64, float64) {
	length_a := DistanceToPoint(x_a*TO_RADIANS, y_a*TO_RADIANS, x_b*TO_RADIANS, y_b*TO_RADIANS)
	length_b := DistanceToPoint(x_a*TO_RADIANS, y_a*TO_RADIANS, x_c*TO_RADIANS, y_c*TO_RADIANS)
	length_c := DistanceToPoint(x_b*TO_RADIANS, y_b*TO_RADIANS, x_c*TO_RADIANS, y_c*TO_RADIANS)

	sp := (length_a + length_b + length_c) / 2

	area := math.Sqrt(sp * (sp - length_a) * (sp - length_b) * (sp - length_c))

	if length_a*length_b*length_c == 0 {
		return 0, 0, 0
	}

	curvature := (4 * area) / (length_a * length_b * length_c)

	radius := 1.0 / curvature

	angle := math.Acos((math.Pow(radius, 2)*2 - math.Pow(length_b, 2)) / (2 * math.Pow(radius, 2)))
	arc_length := radius * angle
	return curvature, arc_length, angle
}
