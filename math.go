package main

import (
	"math"

	"github.com/pkg/errors"
)

var (
	R            = 6373000.0           // approximate radius of earth in meters
	LANE_WIDTH   = 3.7                 // meters
	QUERY_RADIUS = float64(3000)       // meters
	PADDING      = 10 / R * TO_DEGREES // 10 meters in degrees
	TO_RADIANS   = math.Pi / 180
	TO_DEGREES   = 180 / math.Pi
	// TARGET_LAT_ACCEL is used to give an opinionated speed for mtsc.
	// The curvature outputs can be used instead of speed outputs to modify
	// behavior.
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
	currentNodes, err := state.CurrentWay.Way.Nodes()
	if err != nil {
		return []Curvature{}, errors.Wrap(err, "could not read way nodes")
	}
	nextNodes, err := state.NextWay.Way.Nodes()
	if err != nil {
		return []Curvature{}, errors.Wrap(err, "could not read next way nodes")
	}
	secondNodes, err := state.SecondNextWay.Way.Nodes()
	if err != nil {
		return []Curvature{}, errors.Wrap(err, "could not read second way nodes")
	}

	nextLen := nextNodes.Len()
	secondLen := secondNodes.Len()
	num_points := currentNodes.Len()
	if nextLen > 1 {
		num_points += nextLen - 1
	}
	if secondLen > 1 {
		num_points += secondLen - 1
	}
	if num_points < 0 {
		return []Curvature{}, errors.New("not enough nodes to calculate curvatures")
	}
	x_points := make([]float64, num_points)
	y_points := make([]float64, num_points)

	for i := 0; i < currentNodes.Len(); i++ {
		var index int
		if state.CurrentWay.OnWay.IsForward {
			index = i
		} else {
			index = currentNodes.Len() - i - 1
		}
		node := currentNodes.At(index)
		x_points[i] = node.Latitude()
		y_points[i] = node.Longitude()
	}

	if nextNodes.Len() > 1 {
		for i := 0; i < nextNodes.Len()-1; i++ {
			var index int
			if state.NextWay.IsForward {
				index = i
			} else {
				index = nextNodes.Len() - i - 2
			}
			node := nextNodes.At(index)
			x_points[i+currentNodes.Len()-1] = node.Latitude()
			y_points[i+currentNodes.Len()-1] = node.Longitude()
		}
	}

	if secondNodes.Len() > 1 {
		for i := 0; i < secondNodes.Len()-1; i++ {
			var index int
			if state.SecondNextWay.IsForward {
				index = i
			} else {
				index = secondNodes.Len() - i - 2
			}
			node := secondNodes.At(index)
			x_points[i+currentNodes.Len()-1+nextNodes.Len()-1] = node.Latitude()
			y_points[i+currentNodes.Len()-1+nextNodes.Len()-1] = node.Longitude()
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
		velocities[i].Velocity = math.Pow(2.0/curv.Curvature, 1.0/2)
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
		length_a := DistanceToPoint(x_points[i]*TO_RADIANS, y_points[i]*TO_RADIANS, x_points[i+1]*TO_RADIANS, y_points[i+1]*TO_RADIANS)
		length_b := DistanceToPoint(x_points[i]*TO_RADIANS, y_points[i]*TO_RADIANS, x_points[i+2]*TO_RADIANS, y_points[i+2]*TO_RADIANS)
		length_c := DistanceToPoint(x_points[i+1]*TO_RADIANS, y_points[i+1]*TO_RADIANS, x_points[i+2]*TO_RADIANS, y_points[i+2]*TO_RADIANS)

		sp := (length_a + length_b + length_c) / 2

		area := math.Sqrt(sp * (sp - length_a) * (sp - length_b) * (sp - length_c))

		if length_a*length_b*length_c == 0 {
			curvatures[i] = 0
			arc_lengths[i] = 0
			continue
		}

		curvatures[i] = (4 * area) / (length_a * length_b * length_c)

		radius := 1.0 / curvatures[i]

		angle := math.Acos((math.Pow(radius, 2)*2 - math.Pow(length_b, 2)) / (2 * math.Pow(radius, 2)))
		arc_lengths[i] = radius * angle
	}
	return curvatures, arc_lengths, nil
}
