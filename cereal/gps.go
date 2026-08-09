package cereal

import (
	"log/slog"
	"time"

	"pfeifer.dev/mapd/cereal/log"
	ms "pfeifer.dev/mapd/settings"
)

const (
	gpsLocationExternalTimeout = time.Second
	gpsLocationTimeout         = 10 * time.Second
)

type gpsSource uint8

const (
	gpsSourceNone gpsSource = iota
	gpsSourceInternal
	gpsSourceExternal
)

type gpsSample struct {
	data       log.GpsLocationData
	receivedAt time.Time
	usable     bool
}

func (s gpsSample) Healthy(now time.Time, maxAge time.Duration) bool {
	age := now.Sub(s.receivedAt)
	return s.usable && age >= 0 && age < maxAge
}

func (s *gpsSample) Update(data log.GpsLocationData, status SubscriberStatus) {
	s.receivedAt = status.ReceivedAt
	s.usable = status.EventValid && (data.HasFix() || data.Flags()&1 != 0)
	if s.usable {
		s.data = data
	}
}

type GpsSub struct {
	gpsLocation               Subscriber[log.GpsLocationData]
	gpsLocationExternal       Subscriber[log.GpsLocationData]
	gpsLocationSample         gpsSample
	gpsLocationExternalSample gpsSample
	now                       func() time.Time
	source                    gpsSource
}

func (s *GpsSub) Read() (locationData log.GpsLocationData, success bool) {
	internalData, internalUpdated := s.gpsLocation.Read()
	if internalUpdated {
		s.gpsLocationSample.Update(internalData, s.gpsLocation.Status())
	}

	externalData, externalUpdated := s.gpsLocationExternal.Read()
	if externalUpdated {
		s.gpsLocationExternalSample.Update(externalData, s.gpsLocationExternal.Status())
	}

	now := s.nowTime()
	source := gpsSourceNone
	if s.gpsLocationExternalSample.Healthy(now, gpsLocationExternalTimeout) {
		source = gpsSourceExternal
	} else if s.gpsLocationSample.Healthy(now, gpsLocationTimeout) {
		source = gpsSourceInternal
	}

	sourceChanged := source != s.source
	if sourceChanged {
		s.source = source
		if source == gpsSourceExternal {
			slog.Info("Switching to external GPS provider")
		} else if source == gpsSourceInternal {
			slog.Info("Switching to internal GPS provider")
		}
	}

	switch source {
	case gpsSourceExternal:
		if externalUpdated && s.gpsLocationExternalSample.usable || sourceChanged {
			return s.gpsLocationExternalSample.data, true
		}
	case gpsSourceInternal:
		if internalUpdated && s.gpsLocationSample.usable || sourceChanged {
			return s.gpsLocationSample.data, true
		}
	}
	return locationData, false
}

func (s *GpsSub) Fresh(now time.Time) bool {
	switch s.source {
	case gpsSourceExternal:
		return s.gpsLocationExternalSample.Healthy(now, gpsLocationExternalTimeout)
	case gpsSourceInternal:
		return s.gpsLocationSample.Healthy(now, gpsLocationTimeout)
	default:
		return false
	}
}

func (s *GpsSub) nowTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *GpsSub) Close() {
	s.gpsLocation.Sub.Msgq.Close()
	s.gpsLocationExternal.Sub.Msgq.Close()
}

func GetGpsSub() (gpsSub GpsSub) {
	return GpsSub{
		gpsLocation:         NewSubscriber("gpsLocation", GpsLocationReader, true, ms.Settings.SubscriberSettings.ShadowGpsLocation),
		gpsLocationExternal: NewSubscriber("gpsLocationExternal", GpsLocationExternalReader, true, ms.Settings.SubscriberSettings.ShadowGpsLocationExternal),
		now:                 time.Now,
	}
}
