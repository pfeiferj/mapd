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

const (
	mapLoadRetryDelay    = time.Second
	carStateTimeout      = 100 * time.Millisecond
	carStateStaleTimeout = 500 * time.Millisecond
	modelV2Timeout       = 500 * time.Millisecond
)

func main() {
	ms.Settings.Default()                // set defaults so settings not already in param are defaulted
	settingsLoaded := ms.Settings.Load() // try loading settings before cli

	cli.Handle()

	if !settingsLoaded {
		ms.Settings.LoadWithRetries(5)
	}

	state := State{}
	state.Init()
	var lastMapLoadAttempt time.Time

	extendedState := ExtendedState{
		Pub:   cereal.NewPublisher("mapdExtendedOut", cereal.MapdExtendedOutCreator),
		state: &state,
	}
	defer extendedState.Pub.Pub.Msgq.Close()

	pub := cereal.NewPublisher("mapdOut", cereal.MapdOutCreator)
	defer pub.Pub.Msgq.Close()
	state.Publisher = &pub

	sub := cereal.NewSubscriber("mapdIn", cereal.MapdInReader, false, false)
	defer sub.Sub.Msgq.Close()

	cli := cereal.NewSubscriber("mapdCli", cereal.MapdInReader, false, false)
	defer cli.Sub.Msgq.Close()

	gps := cereal.GetGpsSub()
	defer gps.Close()

	car := cereal.NewSubscriber("carState", cereal.CarStateReader, true, ms.Settings.SubscriberSettings.ShadowCarState)
	defer car.Sub.Msgq.Close()

	model := cereal.NewSubscriber("modelV2", cereal.ModelV2Reader, true, ms.Settings.SubscriberSettings.ShadowModelV2)
	defer model.Sub.Msgq.Close()

	selfdriveState := cereal.NewSubscriber("selfdriveState", cereal.SelfdriveStateReader, true, ms.Settings.SubscriberSettings.ShadowSelfdriveState)
	defer selfdriveState.Sub.Msgq.Close()

	for {
		now := time.Now()
		carStatus := car.Status()
		modelStatus := model.Status()
		state.CarValid = carStatus.Healthy(now, carStateStaleTimeout)
		state.GpsValid = gps.Fresh(now)
		state.ModelValid = modelStatus.Healthy(now, modelV2Timeout)
		if !state.GpsValid && state.RouteValid {
			state.ClearRoute()
		}

		err := state.Send(state.CarValid) // send beginning of each loop to ensure it happens at the correct rate
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
		if carStateSuccess && car.Status().EventValid {
			if !carStatus.Healthy(car.Status().ReceivedAt, carStateTimeout) {
				state.Car.UpdateTime.Rebase()
			}
			state.UpdateCarState(carData)
			UpdateCurveSpeed(&state)
		}

		modelData, modelSuccess := model.Read()
		if modelSuccess && model.Status().EventValid {
			if !modelStatus.Healthy(model.Status().ReceivedAt, modelV2Timeout) {
				state.VisionCurveMA.Reset()
			}
			state.VisionCurveSpeed = calcVisionCurveSpeed(modelData, &state)
		}

		selfdriveData, selfdriveSuccess := selfdriveState.Read()
		if selfdriveSuccess {
			ms.Settings.SetPersonality(selfdriveData.Personality())
		}

		location, gpsSuccess := gps.Read()
		if gpsSuccess {
			state.DistanceSinceLastPosition = 0
			state.Position = m.PosFromLocation(location)
			pos := m.PosFromLocation(location)
			box := state.Data.Box()
			mapLoadTime := time.Now()
			if !box.PosInside(pos) || (!state.Data.Loaded && mapLoadTime.Sub(lastMapLoadAttempt) >= mapLoadRetryDelay) {
				state.Data, err = maps.FindWaysAroundPosition(pos)
				lastMapLoadAttempt = mapLoadTime
				if err != nil {
					slog.Debug("", "error", errors.Wrap(err, "Could not find ways around location"))
					state.MapValid = false
					state.ClearRoute()
					continue
				}
			}
			state.MapValid = state.Data.Loaded
			if !state.MapValid {
				state.ClearRoute()
				continue
			}

			state.CurrentWay, err = GetCurrentWay(state.CurrentWay, state.NextWays, &state.Data, location)
			if err != nil {
				slog.Debug("could not get current way", "error", err)
				state.ClearRoute()
				continue
			}
			state.RouteValid = true

			state.NextWays, err = NextWays(location, state.CurrentWay, &state.Data, state.CurrentWay.OnWay.IsForward)
			if err != nil {
				slog.Debug("could not get next way", "error", err)
				state.NextWays = nil
				state.SpeedLimit.NextLimit.Reset()
				state.NextAdvisorySpeed.Reset()
				state.NextHazard.Reset()
			}

			state.Curvatures, err = GetStateCurvatures(&state)
			if err != nil {
				slog.Debug("could not get curvatures from current state", "error", err)
				state.Curvatures = nil
				state.TargetVelocities = nil
				state.MapCurveSpeed = 0
			} else {
				state.TargetVelocities = GetTargetVelocities(state.Curvatures, state.TargetVelocities)
			}
		}

		// send at beginning of next loop
	}
}
