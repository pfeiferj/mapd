package main

import (
	"math"

	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

func calcVtscSpeed(model log.ModelDataV2) float32 {
	xyztData, err := model.OrientationRate()
	if err != nil {
		return 0
	}
	zOrientRate, err := xyztData.Z()
	if err != nil {
		return 0
	}

	xyztData, err = model.Velocity()
	if err != nil {
		return 0
	}
	xVelocity, err := xyztData.X()
	if err != nil {
		return 0
	}

	predictedLatAccels := make([]float32, zOrientRate.Len())
	maxLatA := float32(0)
	vEgo := xVelocity.At(0)
	if vEgo < 0.1 {
		vEgo = 0.1
	}

	for i := range zOrientRate.Len() {
		predictedLatAccels[i] = zOrientRate.At(i) * xVelocity.At(i)
		if predictedLatAccels[i] > maxLatA {
			maxLatA = predictedLatAccels[i]
		}
	}

	maxCurve := maxLatA / (vEgo * vEgo)
	vTarget := float32(math.Sqrt(float64(settings.Settings.VtscTargetLatA / maxCurve)))
	if vTarget < settings.Settings.VtscMinTargetV {
		vTarget = settings.Settings.VtscMinTargetV
	}

	return vTarget
}
