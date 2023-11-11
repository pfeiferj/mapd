package main

import (
	"testing"

	"github.com/bradleyjkemp/cupaloy"
)

func TestDot(t *testing.T) {
	dotProduct := Dot(1.4, 1.5, 1.6, 1.7)

	cupaloy.SnapshotT(t, dotProduct)
}

func TestPointOnLine(t *testing.T) {
	x, y := PointOnLine(1, 1, 3, 3, 1, 2)

	cupaloy.SnapshotT(t, x, y)
}

func TestDistanceToPoint(t *testing.T) {
	distance := DistanceToPoint(39.87597128296241*TO_RADIANS, -83.063094468947*TO_RADIANS, 39.8743989043051*TO_RADIANS, -83.0064776388221*TO_RADIANS)

	cupaloy.SnapshotT(t, "Expected to be ~3 miles or ~4828 meters", distance)
}

func TestVector(t *testing.T) {
	x, y := Vector(39.87597128296241, -83.063094468947, 39.8743989043051, -83.0064776388221)

	cupaloy.SnapshotT(t, x, y)
}

func TestBearing(t *testing.T) {
	north := Bearing(39.89058447975868, -83.02569199443768, 39.898404491651426, -83.02610011832185)
	east := Bearing(39.97639072630465, -83.11918338645518, 39.97031064469578, -82.8450246292918)
	cupaloy.SnapshotT(t, "Should be near 0 (northbound)", north*TO_DEGREES, "Should be near 90 (eastbound)", east*TO_DEGREES)
}
