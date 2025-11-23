package main

import (
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cli"
	"pfeifer.dev/mapd/maps"
	ms "pfeifer.dev/mapd/settings"
	m "pfeifer.dev/mapd/math"
)

func main() {
	settingsLoaded := ms.Settings.Load() // try loading settings before cli

	cli.Handle()

	if !settingsLoaded {
		ms.Settings.LoadWithRetries(5)
	}

	state := State{}
	extendedState := ExtendedState{
		Pub: cereal.NewPublisher("mapdExtendedOut", cereal.MapdExtendedOutCreator),
		state: &state,
	}
	defer extendedState.Pub.Pub.Msgq.Close()

	state.NextSpeedLimitMA.Init(10)
	state.CarStateUpdateTimeMA.Init(100)
	state.VisionCurveMA.Init(20)

	pub := cereal.NewPublisher("mapdOut", cereal.MapdOutCreator)
	defer pub.Pub.Msgq.Close()
	state.Publisher = &pub

	sub := cereal.NewSubscriber("mapdIn", cereal.MapdInReader, false)
	defer sub.Sub.Msgq.Close()

	cli := cereal.NewSubscriber("mapdCli", cereal.MapdInReader, false)
	defer cli.Sub.Msgq.Close()

	gps := cereal.GetGpsSub()
	defer gps.Sub.Msgq.Close()

	car := cereal.NewSubscriber("carState", cereal.CarStateReader, true)
	defer car.Sub.Msgq.Close()

	model := cereal.NewSubscriber("modelV2", cereal.ModelV2Reader, true)
	defer model.Sub.Msgq.Close()

	for {
		err := state.Send() // send beginning of each loop to ensure it happens at the correct rate
		if err != nil {
			slog.Error("Failed to send update", "error", err)
		}
		err = extendedState.Send() // this send is internally rate limited to 1 hz
		if err != nil {
			slog.Error("Failed to send extended update", "error", err)
		}
		time.Sleep(ms.LOOP_DELAY)

		// handle settings inputs from openpilot/cli
		input, inputSuccess := sub.Read()
		if inputSuccess {
			ms.Settings.Handle(input)
		}
		cliInput, cliSuccess := cli.Read()
		if cliSuccess {
			ms.Settings.Handle(cliInput)
		}

		progress, success := ms.Settings.GetDownloadProgress()
		if success {
			extendedState.DownloadProgress = progress
		}

		offlineMaps := maps.ReadOffline(state.Data) // read each loop to get around read safety limits in capnp

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
			box := offlineMaps.Box()
			pos := m.PosFromLocation(location)
			if len(offlineMaps.Ways()) == 0 || !box.PosInside(pos) {
				state.Data, err = maps.FindWaysAroundPosition(pos)
				if err != nil {
					slog.Debug("", "error", errors.Wrap(err, "Could not find ways around location"))
					continue
				}
				offlineMaps = maps.ReadOffline(state.Data)
			}

			state.LastWay = state.CurrentWay
			state.CurrentWay, err = GetCurrentWay(state.CurrentWay, state.NextWays, &offlineMaps, location)
			if err != nil {
				slog.Debug("could not get current way", "error", err)
			}

			state.MaxSpeed = state.CurrentWay.Way.MaxSpeed()
			if state.CurrentWay.OnWay.IsForward && state.CurrentWay.Way.MaxSpeedForward() > 0 {
				state.MaxSpeed = state.CurrentWay.Way.MaxSpeedForward()
			} else if !state.CurrentWay.OnWay.IsForward && state.CurrentWay.Way.MaxSpeedBackward() > 0 {
				state.MaxSpeed = state.CurrentWay.Way.MaxSpeedBackward()
			}

			state.NextWays, err = NextWays(location, state.CurrentWay, &offlineMaps, state.CurrentWay.OnWay.IsForward)
			if err != nil {
				slog.Debug("could not get next way", "error", err)
			}

			state.Curvatures, err = GetStateCurvatures(&state)
			if err != nil {
				slog.Debug("could not get curvatures from current state", "error", err)
			}
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
