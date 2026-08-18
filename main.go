package main

import (
	"log/slog"
	"sync/atomic"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/cli"
	"pfeifer.dev/mapd/maps"
	m "pfeifer.dev/mapd/math"
	ms "pfeifer.dev/mapd/settings"
)

const mapLoadRetryDelay = time.Second

func publishMapdOut(publisher *cereal.Publisher[custom.MapdOut], latest *atomic.Pointer[capnp.Message], stop <-chan struct{}, stopped chan<- struct{}) {
	ticker := time.NewTicker(ms.LOOP_DELAY)
	defer ticker.Stop()
	defer close(stopped)

	for {
		message := latest.Load()
		event, err := log.ReadRootEvent(message)
		if err == nil {
			event.SetLogMonoTime(cereal.GetTime())
			err = publisher.Send(message)
		}
		if err != nil {
			slog.Error("Failed to send update", "error", err)
		}

		select {
		case <-ticker.C:
		case <-stop:
			return
		}
	}
}

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

	latestMapdOut := atomic.Pointer[capnp.Message]{}
	latestMapdOut.Store(state.buildMapdOut(&pub))
	stopMapdOut := make(chan struct{})
	mapdOutStopped := make(chan struct{})
	go publishMapdOut(&pub, &latestMapdOut, stopMapdOut, mapdOutStopped)
	defer func() {
		close(stopMapdOut)
		<-mapdOutStopped
	}()

	for {
		// Build before handoff because State contains unsynchronized lazy getters.
		latestMapdOut.Store(state.buildMapdOut(&pub))

		err := extendedState.Send() // this send is internally rate limited to 1 hz
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

	}
}
