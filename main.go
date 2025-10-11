package main

import (
	"time"
	"log/slog"

	"capnproto.org/go/capnp/v3"
	"github.com/pkg/errors"
)


func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	state := State{}
	sub := getGpsSub()
	defer sub.Sub.Msgq.Close()
	var err error
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
				slog.Debug("", "error", errors.Wrap(err, "Could not find ways around location"));
				continue
			}
		}
		offline = readOffline(state.Data)


		// oldWay := state.CurrentWay.Way
		state.CurrentWay, err = GetCurrentWay(state.CurrentWay, state.NextWays, offline, location)
		if err != nil {
			slog.Debug("", "error", errors.Wrap(err, "Could not get current way"))
			continue
		}
		
		name, _ := state.CurrentWay.Way.Name()
		slog.Debug("current way", "max_speed", state.CurrentWay.Way.MaxSpeed(), "road_name", name)
	}

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

