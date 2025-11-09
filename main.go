package main

import (
	"log/slog"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/cereal/offline"
	"pfeifer.dev/mapd/maps"
	ms "pfeifer.dev/mapd/settings"
	"pfeifer.dev/mapd/utils"
	"pfeifer.dev/mapd/cli"
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

	pub := cereal.GetMapdPub()
	defer pub.Msgq.Close()

	sub := cereal.GetMapdSub("mapdIn")
	defer sub.Sub.Msgq.Close()

	cli := cereal.GetMapdSub("mapdCli")
	defer cli.Sub.Msgq.Close()

	gps := cereal.GetGpsSub()
	defer gps.Sub.Msgq.Close()

	car := cereal.GetCarSub()
	defer car.Sub.Msgq.Close()

	model := cereal.GetModelSub()
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

		offlineMaps := readOffline(state.Data)
		msg := state.ToMessage()

		b, err := msg.Marshal()
		if err != nil {
			slog.Error("Failed to send update", "error", err)
		}
		pub.Send(b)

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
			if !maps.PointInBox(location.Latitude(), location.Longitude(), offlineMaps.MinLat(), offlineMaps.MinLon(), offlineMaps.MaxLat(), offlineMaps.MaxLon()) {
				state.Data, err = maps.FindWaysAroundLocation(location.Latitude(), location.Longitude())
				if err != nil {
					slog.Debug("", "error", errors.Wrap(err, "Could not find ways around location"))
					continue
				}
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

func logOutput(event log.Event, mapdOut custom.MapdOut) {
	//name, _ := mapdOut.WayName()
	//ref, _ := mapdOut.WayRef()
	//hazard, _ := mapdOut.Hazard()
	//slog.Debug("mapdOut",
	//	"valid", event.Valid(),
	//	"wayName", name,
	//	"wayRef", ref,
	//	"speedLimit", mapdOut.SpeedLimit(),
	//	"hazard", hazard,
	//	"advisorySpeed", mapdOut.AdvisorySpeed(),
	//	"oneWay", mapdOut.OneWay(),
	//	"lanes", mapdOut.Lanes(),
	//	"visionCurveSpeed", mapdOut.VisionCurveSpeed(),
	//	"curveSpeed", mapdOut.CurveSpeed(),
	//	"suggestedSpeed", mapdOut.SuggestedSpeed(),
	//)
}

func readOffline(data []uint8) offline.Offline {
	msg, err := capnp.UnmarshalPacked(data)
	utils.Logde(errors.Wrap(err, "could not unmarshal offline data"))
	if err == nil {
		offlineMaps, err := offline.ReadRootOffline(msg)
		utils.Logde(errors.Wrap(err, "could not read offline message"))
		return offlineMaps
	}
	return offline.Offline{}
}

func newOutput() (*capnp.Message, log.Event, custom.MapdOut) {
	arena := capnp.SingleSegment(nil)

	msg, seg, err := capnp.NewMessage(arena)
	if err != nil {
		panic(err)
	}

	event, err := log.NewRootEvent(seg)
	if err != nil {
		panic(err)
	}
	mapdOut, err := event.NewMapdOut()
	if err != nil {
		panic(err)
	}

	return msg, event, mapdOut
}

func calculateNextSpeedLimit(state *State, currentMaxSpeed float64) NextSpeedLimit {
	if len(state.NextWays) == 0 {
		return NextSpeedLimit{}
	}

	// Find the next speed limit change
	cumulativeDistance := 0.0

	if state.CurrentWay.Way.HasNodes() {
		distToEnd, err := DistanceToEndOfWay(state.Location.Latitude(), state.Location.Longitude(), state.CurrentWay.Way, state.CurrentWay.OnWay.IsForward)
		if err == nil && distToEnd > 0 {
			cumulativeDistance = distToEnd - state.CurrentWay.OnWay.Distance.Distance - float64(state.DistanceSinceLastPosition)
		}
	}

	// Look through next ways for speed limit change
	for _, nextWay := range state.NextWays {
		nextMaxSpeed := nextWay.Way.MaxSpeed()
		if nextWay.IsForward && nextWay.Way.MaxSpeedForward() > 0 {
			nextMaxSpeed = nextWay.Way.MaxSpeedForward()
		} else if !nextWay.IsForward && nextWay.Way.MaxSpeedBackward() > 0 {
			nextMaxSpeed = nextWay.Way.MaxSpeedBackward()
		}

		if nextMaxSpeed != currentMaxSpeed && nextMaxSpeed > 0 {
			result := NextSpeedLimit{
				Latitude:   nextWay.StartPosition.Latitude(),
				Longitude:  nextWay.StartPosition.Longitude(),
				Speedlimit: nextMaxSpeed,
				Distance:   cumulativeDistance,
			}

			wayName := RoadName(nextWay.Way)
			if nextMaxSpeed == state.LastSpeedLimitValue && wayName == state.LastSpeedLimitWayName {
				diff := state.LastSpeedLimitDistance - cumulativeDistance
				smoothed_diff := diff
				if state.DistanceSinceLastPosition == 0 {
					smoothed_diff = state.NextSpeedLimitMA.Update(diff)
				}
				result.Distance = state.LastSpeedLimitDistance - smoothed_diff

				slog.Debug("Smoothed speed limit distance",
					"raw_distance", cumulativeDistance,
					"smoothed_distance", result.Distance,
					"last_distance", state.LastSpeedLimitDistance,
					"way", wayName,
				)

			} else {
				state.NextSpeedLimitMA.Reset()
			}
			state.LastSpeedLimitDistance = result.Distance
			state.LastSpeedLimitValue = nextMaxSpeed
			state.LastSpeedLimitWayName = wayName

			return result
		}
		if nextWay.Way.HasNodes() {
			wayDistance, err := calculateWayDistance(nextWay.Way)
			if err == nil {
				cumulativeDistance += wayDistance
			}
		}
	}

	return NextSpeedLimit{}
}

func RoadName(way offline.Way) string {
	name, err := way.Name()
	if err == nil {
		if len(name) > 0 {
			return name
		}
	}
	ref, err := way.Ref()
	if err == nil {
		if len(ref) > 0 {
			return ref
		}
	}
	return ""
}

func calculateWayDistance(way offline.Way) (float64, error) {
	nodes, err := way.Nodes()
	if err != nil {
		return 0, err
	}

	if nodes.Len() < 2 {
		return 0, nil
	}

	totalDistance := 0.0
	for i := range nodes.Len() - 1 {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		distance := DistanceToPoint(
			nodeStart.Latitude()*ms.TO_RADIANS,
			nodeStart.Longitude()*ms.TO_RADIANS,
			nodeEnd.Latitude()*ms.TO_RADIANS,
			nodeEnd.Longitude()*ms.TO_RADIANS,
		)
		totalDistance += distance
	}

	return totalDistance, nil
}
