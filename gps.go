package main

import (
	"github.com/pfeiferj/gomsgq"
	"time"
	"log/slog"

	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal/log"
)


type GpsSubscriber struct {
  Sub gomsgq.MsgqSubscriber
	Read func()(location log.GpsLocationData, success bool)

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
		panic(err)
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
	return location, false
}

func getGpsSub() (gpsSub GpsSubscriber) {
	msgq := gomsgq.Msgq{}
	err := msgq.Init("gpsLocation", DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	sub := gomsgq.MsgqSubscriber{}
	sub.Init(msgq)

	msgqExt := gomsgq.Msgq{}
	err = msgqExt.Init("gpsLocationExternal", DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	subExt := gomsgq.MsgqSubscriber{}
	subExt.Init(msgqExt)

	for {
		time.Sleep(LOOP_DELAY)

		if(sub.Ready()) {
			err, err2 := subExt.Msgq.Close()
			if (err != nil) {
				panic(err)
			}
			if err2 != nil { 
				panic(err2)
			}
			slog.Info("Using gpsLocation");
			gpsSub.Sub = sub
			gpsSub.Read = gpsSub.readGpsLocation
			return gpsSub
		}

		if(subExt.Ready()) {
			slog.Info("Using gpsLocationExternal");
			err, err2 := sub.Msgq.Close()
			if (err != nil) {
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
}
