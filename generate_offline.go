package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"

	"capnproto.org/go/capnp/v3"
	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
)

type TmpNode struct {
	Latitude  float64
	Longitude float64
}
type TmpWay struct {
	Name     string
	Ref      string
	MaxSpeed float64
	Index    int32
	MinLat   float64
	MinLon   float64
	MaxLat   float64
	MaxLon   float64
	Nodes    []TmpNode
}

type Area struct {
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
	Ways   []TmpWay
}

var GROUP_AREA_BOX_DEGREES = 2
var AREA_BOX_DEGREES = 1.0 / 3 // Must be 1.0 divided by an integer number
var WAYS_PER_FILE = 2000

func GetBaseOpPath() string {
	exists, err := Exists("/data/openpilot")
	check(err)
	if exists {
		return "/data/openpilot"
	} else {
		return "."
	}
}

var BOUNDS_DIR = fmt.Sprintf("%s/offline", GetBaseOpPath())

func EnsureOfflineMapsDirectories() {
	err := os.MkdirAll(BOUNDS_DIR, 0775)
	loge(err)
}

// Creates a file for a specific bounding box
func GenerateBoundsFileName(minLat float64, minLon float64, maxLat float64, maxLon float64) string {
	group_lat_directory := int(math.Floor(minLat/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	group_lon_directory := int(math.Floor(minLon/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	dir := fmt.Sprintf("%s/%d/%d", BOUNDS_DIR, group_lat_directory, group_lon_directory)
	err := os.MkdirAll(dir, 0775)
	check(err)
	return fmt.Sprintf("%s/%f_%f_%f_%f", dir, minLat, minLon, maxLat, maxLon)
}

// Checks if two bounding boxes intersect
func Overlapping(axMin float64, ayMin float64, axMax float64, ayMax float64, bxMin float64, byMin float64, bxMax float64, byMax float64) bool {
	intersect := !(axMin > bxMax || axMax < bxMin || ayMin > byMax || ayMax < byMin)
	aMinInside := PointInBox(axMin, ayMin, bxMin, byMin, bxMax, byMax)
	bMinInside := PointInBox(bxMin, byMin, axMin, ayMin, axMax, ayMax)
	aMaxInside := PointInBox(axMax, ayMax, bxMin, byMin, bxMax, byMax)
	bMaxInside := PointInBox(bxMax, byMax, axMin, ayMin, axMax, ayMax)
	return intersect || aMinInside || bMinInside || aMaxInside || bMaxInside
}

// Generates bounding boxes for storing ways
func GenerateAreas() []Area {

	areas := make([]Area, int((361/AREA_BOX_DEGREES)*(181/AREA_BOX_DEGREES)))
	index := 0
	for i := float64(-90); i < 90; i += AREA_BOX_DEGREES {
		for j := float64(-180); j < 180; j += AREA_BOX_DEGREES {
			a := &areas[index]
			a.MinLat = i
			a.MinLon = j
			a.MaxLat = i + AREA_BOX_DEGREES
			a.MaxLon = j + AREA_BOX_DEGREES
			index += 1
		}
	}
	return areas
}

func GenerateOffline() {
	fmt.Println("Generating Offline Map")
	EnsureOfflineMapsDirectories()
	file, err := os.Open("./map.osm.pbf")
	check(err)
	defer file.Close()

	// The third parameter is the number of parallel decoders to use.
	scanner := osmpbf.New(context.Background(), file, runtime.GOMAXPROCS(-1))
	scanner.SkipRelations = true
	defer scanner.Close()

	scannedWays := []TmpWay{}
	areas := GenerateAreas()
	index := 0
	allMinLat := float64(90)
	allMinLon := float64(180)
	allMaxLat := float64(-90)
	allMaxLon := float64(-180)

	println("Scanning Ways")
	for scanner.Scan() {
		var way *osm.Way
		switch o := scanner.Object(); o.(type) {
		case *osm.Way:
			way = o.(*osm.Way)
		default:
			way = nil
		}
		if way != nil && len(way.Nodes) > 1 {
			tags := way.TagMap()
			tmpWay := TmpWay{
				Nodes:    make([]TmpNode, len(way.Nodes)),
				Name:     tags["name"],
				Ref:      tags["ref"],
				Index:    int32(index),
				MaxSpeed: ParseMaxSpeed(tags["maxspeed"]),
			}
			index++

			minLat := float64(90)
			minLon := float64(180)
			maxLat := float64(-90)
			maxLon := float64(-180)
			for i, n := range way.Nodes {
				if n.Lat < minLat {
					minLat = n.Lat
				}
				if n.Lon < minLon {
					minLon = n.Lon
				}
				if n.Lat > maxLat {
					maxLat = n.Lat
				}
				if n.Lon > maxLon {
					maxLon = n.Lon
				}
				tmpWay.Nodes[i].Latitude = n.Lat
				tmpWay.Nodes[i].Longitude = n.Lon
			}
			tmpWay.MinLat = minLat
			tmpWay.MinLon = minLon
			tmpWay.MaxLat = maxLat
			tmpWay.MaxLon = maxLon
			if minLat < allMinLat {
				allMinLat = minLat
			}
			if minLon < allMinLon {
				allMinLon = minLon
			}
			if maxLat > allMaxLat {
				allMaxLat = maxLat
			}
			if maxLon > allMaxLon {
				allMaxLon = maxLon
			}
			scannedWays = append(scannedWays, tmpWay)
		}
	}

	println("Finding Bounds")
	for _, area := range areas {
		haveWays := Overlapping(allMinLat, allMinLon, allMaxLat, allMaxLon, area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
		if !haveWays {
			continue
		}

		arena := capnp.MultiSegment([][]byte{})
		msg, seg, err := capnp.NewMessage(arena)
		check(err)
		rootOffline, err := NewRootOffline(seg)
		check(err)

		for _, way := range scannedWays {
			overlaps := Overlapping(way.MinLat, way.MinLon, way.MaxLat, way.MaxLon, area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
			if overlaps {
				area.Ways = append(area.Ways, way)
			}
		}

		println("Writing Area")
		ways, err := rootOffline.NewWays(int32(len(area.Ways)))
		check(err)
		for i, way := range area.Ways {
			w := ways.At(i)
			w.SetMinLat(way.MinLat)
			w.SetMinLon(way.MinLon)
			w.SetMaxLat(way.MaxLat)
			w.SetMaxLon(way.MaxLon)
			err := w.SetName(way.Name)
			check(err)
			err = w.SetRef(way.Ref)
			check(err)
			w.SetMaxSpeed(way.MaxSpeed)
			nodes, err := w.NewNodes(int32(len(way.Nodes)))
			check(err)
			for j, node := range way.Nodes {
				n := nodes.At(j)
				n.SetLatitude(node.Latitude)
				n.SetLongitude(node.Longitude)
			}
		}

		data, err := msg.MarshalPacked()
		check(err)
		err = os.WriteFile(GenerateBoundsFileName(area.MinLat, area.MinLon, area.MaxLat, area.MaxLon), data, 0644)
		check(err)

	}

	fmt.Println("Done Generating Offline Map")
}

func PointInBox(ax float64, ay float64, bxMin float64, byMin float64, bxMax float64, byMax float64) bool {
	return ax > bxMin && ax < bxMax && ay > byMin && ay < byMax
}

func FindWaysAroundLocation(lat float64, lon float64) (Offline, Area, error) {
	areas := GenerateAreas()
	offline := Offline{}
	area := Area{}
	for _, area := range areas {
		inBox := PointInBox(lat, lon, area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
		if inBox {
			boundsName := GenerateBoundsFileName(area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
			data, err := os.ReadFile(boundsName)
			if err != nil {
				return offline, area, err
			}
			msg, err := capnp.UnmarshalPacked(data)
			if err != nil {
				return offline, area, err
			}
			offline, err := ReadRootOffline(msg)
			if err != nil {
				return offline, area, err
			}
			return offline, area, nil
		}
	}
	return offline, area, nil
}
