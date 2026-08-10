package math

import (
	m "math"
)

type Curvature struct {
	Curvature, ArcLength, Angle float64
	Pos                         Position
}

func CalculateCurvature(a Position, b Position, c Position) Curvature {
	lengthA := a.distanceTo(b)
	lengthB := a.distanceTo(c)
	lengthC := b.distanceTo(c)

	lengthProd := lengthA * lengthB * lengthC
	if lengthProd == 0 {
		return Curvature{Pos: b}
	}

	res := Curvature{Pos: b}

	x, y, z := lengthA, lengthB, lengthC
	if x < y {
		x, y = y, x
	}
	if x < z {
		x, z = z, x
	}
	if y < z {
		y, z = z, y
	}

	areaProduct := (x + (y + z)) * (z - (x - y)) * (z + (x - y)) * (x + (y - z))
	if areaProduct <= 0 {
		res.ArcLength = lengthB
		return res
	}

	area := 0.25 * m.Sqrt(areaProduct)
	res.Curvature = 4 * area / lengthProd

	res.Angle = 2 * m.Asin(m.Min(1, lengthB*res.Curvature/2))
	res.ArcLength = res.Angle / res.Curvature

	return res
}
