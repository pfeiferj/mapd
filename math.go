package main

import (
	"math"
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

func DistanceToPoint(ax float64, ay float64, bx float64, by float64) float64 {
	a := math.Sin((bx-ax)/2)*math.Sin((bx-ax)/2) + math.Cos(ax)*math.Cos(bx)*math.Sin((by-ay)/2)*math.Sin((by-ay)/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c // in metres
}
