package cereal

import (
	"log/slog"
	"time"

	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

func GetGpsSub() (gpsSub Subscriber[log.GpsLocationData]) {
	sub := NewSubscriber("gpsLocation", GpsLocationReader)
	subExt := NewSubscriber("gpsLocationExternal", GpsLocationExternalReader)

	for range 60 {
		time.Sleep(settings.LOOP_DELAY)

		if sub.Sub.Ready() {
			err, err2 := subExt.Sub.Msgq.Close()
			if err != nil {
				panic(err)
			}
			if err2 != nil {
				panic(err2)
			}
			slog.Info("Using gpsLocation")
			gpsSub = sub
			return gpsSub
		}

		if subExt.Sub.Ready() {
			slog.Info("Using gpsLocationExternal")
			err, err2 := sub.Sub.Msgq.Close()
			if err != nil {
				panic(err)
			}
			if err2 != nil {
				panic(err2)
			}
			gpsSub = subExt
			return gpsSub
		}
	}
	err, err2 := subExt.Sub.Msgq.Close()
	if err != nil {
		panic(err)
	}
	if err2 != nil {
		panic(err2)
	}
	slog.Info("Using gpsLocation")
	gpsSub = sub
	return gpsSub
}
