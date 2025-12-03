package main

import (
	"pfeifer.dev/mapd/cereal/car"
	ms "pfeifer.dev/mapd/settings"
	"pfeifer.dev/mapd/utils"
)

type CarState struct {
	SetSpeed   utils.TrackedState[float32]
	VEgo       float32
	AEgo       float32
	VCruise    float32
	GasPressed bool
	UpdateTime utils.UpdateTracker
}

func (c *CarState) Update(carData car.CarState) {
	c.SetSpeed.Update(carData.VCruise() * ms.KPH_TO_MS)
	c.VEgo = carData.VEgo()
	c.AEgo = carData.AEgo()
	c.VCruise = carData.VCruise()
	c.GasPressed = carData.GasPressed()
}

func (c *CarState) Init() {
	c.SetSpeed.Equal = utils.Float32Compare
	c.SetSpeed.Null = utils.Float32Null
	c.SetSpeed.AllowNullLastValue = true
	c.UpdateTime.Init(100)
}
