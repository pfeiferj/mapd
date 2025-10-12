package main

import (
	"log/slog"
	"math"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/pfeiferj/gomsgq"
	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
)

func main() {
	var err error
	slog.SetLogLoggerLevel(slog.LevelDebug)
	state := State{}

	msgq := gomsgq.Msgq{}
	err = msgq.Init("mapdOut", DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	pub := gomsgq.MsgqPublisher{}
	pub.Init(msgq)

	sub := getGpsSub()
	defer sub.Sub.Msgq.Close()
	offline := readOffline(state.Data)
	for {
		time.Sleep(LOOP_DELAY)
		location, success := sub.Read()
		if !success {
			continue
		}
		if !PointInBox(location.Latitude(), location.Longitude(), offline.MinLat(), offline.MinLon(), offline.MaxLat(), offline.MaxLon()) {
			state.Data, err = FindWaysAroundLocation(location.Latitude(), location.Longitude())
			if err != nil {
				slog.Debug("", "error", errors.Wrap(err, "Could not find ways around location"))
				continue
			}
		}
		offline = readOffline(state.Data)

		state.LastWay = state.CurrentWay
		state.CurrentWay, err = GetCurrentWay(state.CurrentWay, state.NextWays, offline, location)
		logde(errors.Wrap(err, "could not get current way"))

		state.NextWays, err = NextWays(location, state.CurrentWay, offline, state.CurrentWay.OnWay.IsForward)
		logde(errors.Wrap(err, "could not get next way"))

		state.Curvatures, err = GetStateCurvatures(&state)
		logde(errors.Wrap(err, "could not get curvatures from current state"))
		state.TargetVelocities = GetTargetVelocities(state.Curvatures)

		state.NextSpeedLimit = calculateNextSpeedLimit(&state, state.MaxSpeed)

		msg := state.ToMessage()

		b, err := msg.Marshal()
		if err != nil {
			slog.Error("Failed to send update", "error", err)
		}
		pub.Send(b)
	}
}

func (s *State) ToMessage() *capnp.Message {
	newOutput()

	msg, event, output := newOutput()

	event.SetValid(true)

	name, _ := s.CurrentWay.Way.Name()
	output.SetWayName(name)

	ref, _ := s.CurrentWay.Way.Ref()
	output.SetWayRef(ref)

	roadName := RoadName(s.CurrentWay.Way)
	output.SetRoadName(roadName)

	maxSpeed := s.CurrentWay.Way.MaxSpeed()
	output.SetSpeedLimit(float32(maxSpeed))

	output.SetNextSpeedLimit(float32(s.NextSpeedLimit.Speedlimit))
	output.SetNextSpeedLimitDistance(float32(s.NextSpeedLimit.Distance))

	hazard, _ := s.CurrentWay.Way.Hazard()
	output.SetHazard(hazard)

	advisorySpeed := s.CurrentWay.Way.AdvisorySpeed()
	output.SetAdvisorySpeed(float32(advisorySpeed))

	oneWay := s.CurrentWay.Way.OneWay()
	output.SetOneWay(oneWay)

	lanes := s.CurrentWay.Way.Lanes()
	output.SetLanes(lanes)

	if len(s.Data) > 0 {
		output.SetTileLoaded(true)
	} else {
		output.SetTileLoaded(false)
	}

	output.SetRoadContext(custom.RoadContext(s.CurrentWay.Context))
	output.SetEstimatedRoadWidth(float32(estimateRoadWidth(s.CurrentWay.Way)))

	logOutput(event, output)

	return msg
}

func logOutput(event log.Event, mapdOut custom.MapdOut) {
	name, _ := mapdOut.WayName()
	ref, _ := mapdOut.WayRef()
	hazard, _ := mapdOut.Hazard()
	slog.Debug("mapdOut",
		"valid", event.Valid(),
		"wayName", name,
		"wayRef", ref,
		"speedLimit", mapdOut.SpeedLimit(),
		"hazard", hazard,
		"advisorySpeed", mapdOut.AdvisorySpeed(),
		"oneWay", mapdOut.OneWay(),
		"lanes", mapdOut.Lanes(),
	)
}

func readOffline(data []uint8) Offline {
	msg, err := capnp.UnmarshalPacked(data)
	logde(errors.Wrap(err, "could not unmarshal offline data"))
	if err == nil {
		offline, err := ReadRootOffline(msg)
		logde(errors.Wrap(err, "could not read offline message"))
		return offline
	}
	return Offline{}
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
			cumulativeDistance = distToEnd
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
				smoothedDistance := state.LastSpeedLimitDistance*0.8 + cumulativeDistance*0.2

				if math.Abs(smoothedDistance-state.LastSpeedLimitDistance) < 50 {
					result.Distance = smoothedDistance
				}

				slog.Debug("Smoothed speed limit distance",
					"raw_distance", cumulativeDistance,
					"smoothed_distance", result.Distance,
					"last_distance", state.LastSpeedLimitDistance,
					"way", wayName,
				)

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

func RoadName(way Way) string {
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

func calculateWayDistance(way Way) (float64, error) {
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
			nodeStart.Latitude()*TO_RADIANS,
			nodeStart.Longitude()*TO_RADIANS,
			nodeEnd.Latitude()*TO_RADIANS,
			nodeEnd.Longitude()*TO_RADIANS,
		)
		totalDistance += distance
	}

	return totalDistance, nil
}
