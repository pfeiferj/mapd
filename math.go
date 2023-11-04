package main

import (
	"math"
)

var R = 6373000.0                 // approximate radius of earth in meters
var LANE_WIDTH = 3.7              // meters
var QUERY_RADIUS = float64(3000)  // meters
var PADDING = 10 / R * TO_DEGREES // 10 meters in degrees
var TO_RADIANS = math.Pi / 180
var TO_DEGREES = 180 / math.Pi

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
