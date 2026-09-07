package math

import (
	m "math"
)

type Curvature struct {
	Curvature, ArcLength, Angle float64
	Pos                         Position
}

func CalculateCurvature(a Position, b Position, c Position) Curvature {
	distanceAB := a.distanceTo(b)
	distanceAC := a.distanceTo(c)
	distanceBC := b.distanceTo(c)
	distanceProduct := distanceAB * distanceAC * distanceBC
	if distanceProduct == 0 {
		return Curvature{Pos: b}
	}

	longestSide, middleSide, shortestSide := distanceAB, distanceAC, distanceBC
	if longestSide < middleSide {
		longestSide, middleSide = middleSide, longestSide
	}
	if longestSide < shortestSide {
		longestSide, shortestSide = shortestSide, longestSide
	}
	if middleSide < shortestSide {
		middleSide, shortestSide = shortestSide, middleSide
	}

	// Sorted-side Heron arithmetic reduces cancellation for nearly straight roads.
	areaProduct := (longestSide + (middleSide + shortestSide)) *
		(shortestSide - (longestSide - middleSide)) *
		(shortestSide + (longestSide - middleSide)) *
		(longestSide + (middleSide - shortestSide))
	res := Curvature{Pos: b}
	res.Curvature = m.Sqrt(max(0, areaProduct)) / distanceProduct
	if res.Curvature == 0 {
		res.ArcLength = distanceAC
		return res
	}

	res.Angle = 2 * m.Asin(min(1, distanceAC*res.Curvature/2))
	res.ArcLength = res.Angle / res.Curvature
	return res
}
