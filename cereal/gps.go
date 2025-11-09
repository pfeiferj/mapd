package cereal

import (
	"log/slog"
	"time"

	"github.com/pfeiferj/gomsgq"

	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

type GpsSubscriber struct {
	Sub  gomsgq.MsgqSubscriber
	Read func() (location log.GpsLocationData, success bool)
}

func (g *GpsSubscriber) readGpsLocation() (location log.GpsLocationData, success bool) {
	data := g.Sub.Read()
	if len(data) == 0 {
		return location, false
	}
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return location, false
	}

	event, err := log.ReadRootEvent(msg)
	if err != nil {
		return location, false
	}

	location, err = event.GpsLocation()
	if err != nil {
		return location, false
	}
	return location, true
}

func (g *GpsSubscriber) readGpsLocationExternal() (location log.GpsLocationData, success bool) {
	data := g.Sub.Read()
	if len(data) == 0 {
		return location, false
	}
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return location, false
	}

	event, err := log.ReadRootEvent(msg)
	if err != nil {
		return location, false
	}

	location, err = event.GpsLocationExternal()
	if err != nil {
		panic(err)
	}
	return location, true
}

func GetGpsSub() (gpsSub GpsSubscriber) {
	msgq := gomsgq.Msgq{}
	err := msgq.Init("gpsLocation", settings.DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	sub := gomsgq.MsgqSubscriber{}
	sub.Init(msgq)

	msgqExt := gomsgq.Msgq{}
	err = msgqExt.Init("gpsLocationExternal", settings.DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	subExt := gomsgq.MsgqSubscriber{}
	subExt.Init(msgqExt)

	for range 60 {
		time.Sleep(settings.LOOP_DELAY)

		if sub.Ready() {
			err, err2 := subExt.Msgq.Close()
			if err != nil {
				panic(err)
			}
			if err2 != nil {
				panic(err2)
			}
			slog.Info("Using gpsLocation")
			gpsSub.Sub = sub
			gpsSub.Read = gpsSub.readGpsLocation
			return gpsSub
		}

		if subExt.Ready() {
			slog.Info("Using gpsLocationExternal")
			err, err2 := sub.Msgq.Close()
			if err != nil {
				panic(err)
			}
			if err2 != nil {
				panic(err2)
			}
			gpsSub.Sub = subExt
			gpsSub.Read = gpsSub.readGpsLocationExternal
			return gpsSub
		}
	}
	err, err2 := subExt.Msgq.Close()
	if err != nil {
		panic(err)
	}
	if err2 != nil {
		panic(err2)
	}
	slog.Info("Using gpsLocation")
	gpsSub.Sub = sub
	gpsSub.Read = gpsSub.readGpsLocation
	return gpsSub
}
