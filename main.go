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

const mapLoadRetryDelay = time.Second

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
	extendedState.Pub.StartAutoPublish(time.Second)
	defer extendedState.Pub.Stop()
	defer extendedState.Pub.Pub.Msgq.Close()

	pub := cereal.NewPublisher("mapdOut", cereal.MapdOutCreator)
	pub.StartAutoPublish(ms.LOOP_DELAY)
	defer pub.Stop()
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

	lastLoopTime := time.Now()

	for {
		lastLoopDuration := time.Since(lastLoopTime)
		time.Sleep(max(ms.LOOP_DELAY-lastLoopDuration, 0))
		now := time.Now()
		extendedState.LoopRate.Add(now.Sub(lastLoopTime))
		lastLoopTime = now

		// Publish() only queues the latest state; the publisher's own
		// auto-publish loop owns the actual on-wire send rate, resending
		// the last message with a fresh timestamp if this loop stalls.
		err := state.Send()
		if err != nil {
			slog.Error("Failed to send update", "error", err)
		}
		err = extendedState.Send() // rebuilt at most once per second; publisher handles the send cadence
		if err != nil {
			slog.Error("Failed to send extended update", "error", err)
		}

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
