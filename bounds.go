package main

import (
	"github.com/serjvanilla/go-overpass"
)

type Box struct {
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
}

func Bounds(nodes []*overpass.Node) Box {
	box := Box{
		MinLat: 90,
		MinLon: 180,
		MaxLat: -90,
		MaxLon: -180,
	}

	for _, node := range nodes {
		if node.Lat > box.MaxLat {
			box.MaxLat = node.Lat
		}
		if node.Lat < box.MinLat {
			box.MinLat = node.Lat
		}
		if node.Lon > box.MaxLon {
			box.MaxLon = node.Lon
		}
		if node.Lon < box.MinLon {
			box.MinLon = node.Lon
		}
	}

	box.MaxLat = box.MaxLat + PADDING
	box.MaxLon = box.MaxLon + PADDING
	box.MinLat = box.MinLat - PADDING
	box.MinLon = box.MinLon - PADDING
	return box
}
