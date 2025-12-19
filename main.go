package main

import (
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cli"
	"pfeifer.dev/mapd/maps"
	m "pfeifer.dev/mapd/math"
	ms "pfeifer.dev/mapd/settings"
)

func main() {
	ms.Settings.Default() // set defaults so settings not already in param are defaulted
	settingsLoaded := ms.Settings.Load() // try loading settings before cli

	cli.Handle()

	if !settingsLoaded {
		ms.Settings.LoadWithRetries(5)
	}

	state := State{}
	state.Init()

	extendedState := ExtendedState{
		Pub:   cereal.NewPublisher("mapdExtendedOut", cereal.MapdExtendedOutCreator),
		state: &state,
	}
	defer extendedState.Pub.Pub.Msgq.Close()

	pub := cereal.NewPublisher("mapdOut", cereal.MapdOutCreator)
	defer pub.Pub.Msgq.Close()
	state.Publisher = &pub

	sub := cereal.NewSubscriber("mapdIn", cereal.MapdInReader, false)
	defer sub.Sub.Msgq.Close()

	cli := cereal.NewSubscriber("mapdCli", cereal.MapdInReader, false)
	defer cli.Sub.Msgq.Close()

	gps := cereal.GetGpsSub()
	defer gps.Close()

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

		carData, carStateSuccess := car.Read()
		if carStateSuccess {
			state.UpdateCarState(carData)
			UpdateCurveSpeed(&state)
		}

		modelData, modelSuccess := model.Read()
		if modelSuccess {
			state.VisionCurveSpeed = calcVisionCurveSpeed(modelData, &state)
		}

		location, gpsSuccess := gps.Read()
		if gpsSuccess {
			state.DistanceSinceLastPosition = 0
			state.Position = m.PosFromLocation(location)
			box := state.Data.Box()
			pos := m.PosFromLocation(location)
			if len(state.Data.Ways()) == 0 || !box.PosInside(pos) {
				state.Data, err = maps.FindWaysAroundPosition(pos)
				if err != nil {
					slog.Debug("", "error", errors.Wrap(err, "Could not find ways around location"))
					continue
				}
			}

			state.CurrentWay, err = GetCurrentWay(state.CurrentWay, state.NextWays, &state.Data, location)
			if err != nil {
				slog.Debug("could not get current way", "error", err)
			}

			state.NextWays, err = NextWays(location, state.CurrentWay, &state.Data, state.CurrentWay.OnWay.IsForward)
			if err != nil {
				slog.Debug("could not get next way", "error", err)
			}

			state.Curvatures, err = GetStateCurvatures(&state)
			if err != nil {
				slog.Debug("could not get curvatures from current state", "error", err)
			}
			state.TargetVelocities = GetTargetVelocities(state.Curvatures, state.TargetVelocities)
		}

		// send at beginning of next loop
	}
}
