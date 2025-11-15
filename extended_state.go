package main

import (
	"encoding/json"
	"log/slog"
	"time"

	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/offline"
	ms "pfeifer.dev/mapd/settings"
)


type ExtendedState struct {
	DownloadProgress ms.DownloadProgress
	Pub cereal.Publisher[custom.MapdExtendedOut]
	lastSend time.Time
	state *State
}

func (s *ExtendedState) Send() error {
	if time.Since(s.lastSend) > 1 * time.Second {
		s.lastSend = time.Now()
		msg, out := s.Pub.NewMessage(true)
		s.setDownloadProgress(out)
		s.setSettings(out)
		s.setPath(out)
		return s.Pub.Send(msg)
	}
	return nil
}

func (s *ExtendedState) setPath(out custom.MapdExtendedOut) {
	nodes, err := s.state.CurrentWay.Way.Nodes()
	if err != nil {
		slog.Warn("could not get current way nodes")
		return
	}
	num_points := nodes.Len()
	all_nodes := []capnp.StructList[offline.Coordinates]{nodes}
	all_nodes_direction := []bool{s.state.CurrentWay.OnWay.IsForward}
	for _, nextWay := range s.state.NextWays {
		nwNodes, err := nextWay.Way.Nodes()
		if err != nil {
			continue
		}
		if nwNodes.Len() > 0 {
			num_points += nwNodes.Len() - 1
		}
		all_nodes = append(all_nodes, nwNodes)
		all_nodes_direction = append(all_nodes_direction, nextWay.IsForward)
	}

	path, err := out.NewPath(int32(num_points))
	if err != nil {
		slog.Warn("failed to create path in extended state")
		return
	}

	all_nodes_idx := 0
	nodes_idx := 0
	for i := 0; i < num_points; i++ {
		var index int
		forward := all_nodes_direction[all_nodes_idx]
		if forward {
			index = nodes_idx
			if all_nodes_idx > 0 {
				index += 1
			}
		} else {
			index = all_nodes[all_nodes_idx].Len() - nodes_idx - 1
			if all_nodes_idx > 0 {
				index -= 1
			}
		}
		node := all_nodes[all_nodes_idx].At(index)
		point := path.At(i)
		point.SetLatitude(node.Latitude())
		point.SetLongitude(node.Longitude())
		nodes_idx += 1
		if nodes_idx == all_nodes[all_nodes_idx].Len() || (nodes_idx == all_nodes[all_nodes_idx].Len()-1 && all_nodes_idx > 0) {
			all_nodes_idx += 1
			nodes_idx = 0
		}
	}
	point_idx := 0
	for _, curvature := range s.state.Curvatures {
		for ; point_idx < path.Len(); point_idx++ {
			point := path.At(point_idx)
			if curvature.Latitude == point.Latitude() && curvature.Longitude == point.Longitude() {
				point.SetCurvature(float32(curvature.Curvature))
				point_idx++
				break
			}
		}
	}
	point_idx = 0
	for _, velocity := range s.state.TargetVelocities {
		for ; point_idx < path.Len(); point_idx++ {
			point := path.At(point_idx)
			if velocity.Latitude == point.Latitude() && velocity.Longitude == point.Longitude() {
				point.SetTargetVelocity(float32(velocity.Velocity))
				point_idx++
				break
			}
		}
	}
}

func (s *ExtendedState) setSettings(out custom.MapdExtendedOut) {
	b, err := json.Marshal(ms.Settings)
	if err != nil {
		slog.Warn("failed to marshal settings for extended state")
		return
	}
	if err := out.SetSettings(string(b)); err != nil {
		slog.Warn("failed to set settings in extended state")
	}
}

func (s *ExtendedState) setDownloadProgress(out custom.MapdExtendedOut) {
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
}

