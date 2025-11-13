package main

import (
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cli"
	"pfeifer.dev/mapd/maps"
	ms "pfeifer.dev/mapd/settings"
	"pfeifer.dev/mapd/utils"
)

func main() {
	settingsLoaded := ms.Settings.Load() // try loading settings before cli

	cli.Handle()

	if !settingsLoaded {
		ms.Settings.LoadWithRetries(5)
	}

	state := State{}

	state.NextSpeedLimitMA.Init(10)
	state.CarStateUpdateTimeMA.Init(100)
	state.VisionCurveMA.Init(20)

	pub := cereal.NewPublisher("mapdOut", cereal.MapdOutCreator)
	defer pub.Pub.Msgq.Close()
	state.Publisher = &pub

	sub := cereal.NewSubscriber("mapdIn", cereal.MapdInReader)
	defer sub.Sub.Msgq.Close()

	cli := cereal.NewSubscriber("mapdCli", cereal.MapdInReader)
	defer cli.Sub.Msgq.Close()

	gps := cereal.GetGpsSub()
	defer gps.Sub.Msgq.Close()

	car := cereal.NewSubscriber("carState", cereal.CarStateReader)
	defer car.Sub.Msgq.Close()

	model := cereal.NewSubscriber("modelV2", cereal.ModelV2Reader)
	defer model.Sub.Msgq.Close()

	for {
		input, inputSuccess := sub.Read()
		if inputSuccess {
			ms.Settings.Handle(input)
		}
		cliInput, cliSuccess := cli.Read()
		if cliSuccess {
			ms.Settings.Handle(cliInput)
		}

		offlineMaps := maps.ReadOffline(state.Data)
		msg := state.ToMessage()

		err := pub.Send(msg)
		if err != nil {
			slog.Error("Failed to send update", "error", err)
		}

		time.Sleep(ms.LOOP_DELAY)

		carData, carStateSuccess := car.Read()
		if carStateSuccess {
			state.UpdateCarState(carData)
			state.TimeLastCarState = time.Now()
		}

		modelData, modelSuccess := model.Read()
		if modelSuccess {
			state.TimeLastModel = time.Now()
			state.VisionCurveSpeed = calcVisionCurveSpeed(modelData, &state)
		}

		location, gpsSuccess := gps.Read()
		if gpsSuccess {
			state.TimeLastPosition = time.Now()
			state.DistanceSinceLastPosition = 0
			state.Location = location
			if !offlineMaps.HasWays() || !maps.PointInBox(location.Latitude(), location.Longitude(), offlineMaps.MinLat(), offlineMaps.MinLon(), offlineMaps.MaxLat(), offlineMaps.MaxLon()) {
				state.Data, err = maps.FindWaysAroundLocation(location.Latitude(), location.Longitude())
				if err != nil {
					slog.Debug("", "error", errors.Wrap(err, "Could not find ways around location"))
					continue
				}
				offlineMaps = maps.ReadOffline(state.Data)
			}

			state.LastWay = state.CurrentWay
			state.CurrentWay, err = GetCurrentWay(state.CurrentWay, state.NextWays, offlineMaps, location)
			utils.Logde(errors.Wrap(err, "could not get current way"))

			state.MaxSpeed = state.CurrentWay.Way.MaxSpeed()
			if state.CurrentWay.OnWay.IsForward && state.CurrentWay.Way.MaxSpeedForward() > 0 {
				state.MaxSpeed = state.CurrentWay.Way.MaxSpeedForward()
			} else if !state.CurrentWay.OnWay.IsForward && state.CurrentWay.Way.MaxSpeedBackward() > 0 {
				state.MaxSpeed = state.CurrentWay.Way.MaxSpeedBackward()
			}

			state.NextWays, err = NextWays(location, state.CurrentWay, offlineMaps, state.CurrentWay.OnWay.IsForward)
			utils.Logde(errors.Wrap(err, "could not get next way"))

			state.Curvatures, err = GetStateCurvatures(&state)
			utils.Logde(errors.Wrap(err, "could not get curvatures from current state"))
			state.TargetVelocities = GetTargetVelocities(state.Curvatures)
			state.NextSpeedLimit = calculateNextSpeedLimit(&state, state.MaxSpeed)
		}

		if gpsSuccess || carStateSuccess {
			UpdateCurveSpeed(&state)
			state.NextSpeedLimit = calculateNextSpeedLimit(&state, state.MaxSpeed)
		}

		// send at beginning of next loop
	}
}
