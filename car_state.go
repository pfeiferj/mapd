package main

import (
	"pfeifer.dev/mapd/cereal/car"
	ms "pfeifer.dev/mapd/settings"
	"pfeifer.dev/mapd/utils"
)

type CarState struct {
	SetSpeed   utils.Float32Tracker
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
	c.SetSpeed.AllowNullLastValue = true
	c.UpdateTime.Init(100)
}
