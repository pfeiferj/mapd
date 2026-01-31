package cereal

import (
	"log/slog"

	"pfeifer.dev/mapd/cereal/log"
)

type GpsSub struct {
	gpsLocation Subscriber[log.GpsLocationData]
	gpsLocationExternal Subscriber[log.GpsLocationData]
	useExt bool
}

func (s *GpsSub) Read() (locationData log.GpsLocationData, success bool) {
	if s.useExt {
		return s.gpsLocationExternal.Read()
	} else {
		locationData, success = s.gpsLocationExternal.Read()
		if success {
			s.useExt = true
			slog.Info("Found gpsLocationExternal, switching to external GPS provider")
			return locationData, success
		}
	}
	return s.gpsLocation.Read()
}

func (s *GpsSub) Close() {
	s.gpsLocation.Sub.Msgq.Close()
	s.gpsLocationExternal.Sub.Msgq.Close()
}

func GetGpsSub() (gpsSub GpsSub) {
	return GpsSub{
		gpsLocation: NewSubscriber("gpsLocation", GpsLocationReader, true, false),
		gpsLocationExternal: NewSubscriber("gpsLocationExternal", GpsLocationExternalReader, true, false),
		useExt: false,
	}
}
