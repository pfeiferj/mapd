package main

import (
	"time"

	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cereal/custom"
	ms "pfeifer.dev/mapd/settings"
)


type ExtendedState struct {
	DownloadProgress ms.DownloadProgress
	Pub cereal.Publisher[custom.MapdExtendedOut]
	lastSend time.Time
}

func (s *ExtendedState) Send() error {
	if time.Since(s.lastSend) > 1 * time.Second {
		s.lastSend = time.Now()
		return s.Pub.Send(s.downloadProgressToMessage())
	}
	return nil
}

func (s *ExtendedState) downloadProgressToMessage() *capnp.Message {
	msg, out := s.Pub.NewMessage(true)
	p, err := out.NewDownloadProgress()
	if err != nil {
		panic(err)
	}
	p.SetActive(s.DownloadProgress.Active)
	p.SetTotalFiles(uint32(s.DownloadProgress.TotalFiles))
	p.SetDownloadedFiles(uint32(s.DownloadProgress.DownloadedFiles))
	l, err := p.NewLocations(int32(len(s.DownloadProgress.LocationsToDownload)))
	if err != nil {
		panic(err)
	}
	for i, location := range s.DownloadProgress.LocationsToDownload {
		err := l.Set(i, location)
		if err != nil {
			panic(err)
		}
	}
	ld, err := p.NewLocationDetails(int32(len(s.DownloadProgress.LocationDetails)))
	if err != nil {
		panic(err)
	}
	idx := 0
	for location, locationDetails := range s.DownloadProgress.LocationDetails {
		d := ld.At(idx)
		err := d.SetLocation(location)
		if err != nil {
			panic(err)
		}
		d.SetDownloadedFiles(uint32(locationDetails.DownloadedFiles))
		d.SetTotalFiles(uint32(locationDetails.TotalFiles))
		idx++
	}
	return msg
}

