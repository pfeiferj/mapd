package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"

	"capnproto.org/go/capnp/v3"
	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

type TmpNode struct {
	Latitude  float64
	Longitude float64
}
type TmpWay struct {
	Name             string
	Ref              string
	Hazard           string
	MaxSpeed         float64
	MaxSpeedAdvisory float64
	Lanes            uint8
	MinLat           float64
	MinLon           float64
	MaxLat           float64
	MaxLon           float64
	Nodes            []TmpNode
}

type Area struct {
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
	Ways   []TmpWay
}

var (
	GROUP_AREA_BOX_DEGREES = 2
	AREA_BOX_DEGREES       = float64(1.0 / 4) // Must be 1.0 divided by an integer number
	OVERLAP_BOX_DEGREES    = float64(0.01)
	WAYS_PER_FILE          = 2000
)

func GetBaseOpPath() string {
	exists, err := Exists("/data/media/0")
	logde(err)
	if exists {
		return "/data/media/0/osm"
	} else {
		return "."
	}
}

var BOUNDS_DIR = fmt.Sprintf("%s/offline", GetBaseOpPath())

func EnsureOfflineMapsDirectories() {
	err := os.MkdirAll(BOUNDS_DIR, 0o775)
	logwe(err)
}

// Creates a file for a specific bounding box
func GenerateBoundsFileName(minLat float64, minLon float64, maxLat float64, maxLon float64) string {
	group_lat_directory := int(math.Floor(minLat/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	group_lon_directory := int(math.Floor(minLon/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	dir := fmt.Sprintf("%s/%d/%d", BOUNDS_DIR, group_lat_directory, group_lon_directory)
	return fmt.Sprintf("%s/%f_%f_%f_%f", dir, minLat, minLon, maxLat, maxLon)
}

// Creates a file for a specific bounding box
func CreateBoundsDir(minLat float64, minLon float64, maxLat float64, maxLon float64) error {
	group_lat_directory := int(math.Floor(minLat/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	group_lon_directory := int(math.Floor(minLon/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	dir := fmt.Sprintf("%s/%d/%d", BOUNDS_DIR, group_lat_directory, group_lon_directory)
	err := os.MkdirAll(dir, 0o775)
	return errors.Wrap(err, "could not create bounds directory")
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

func GenerateOffline(minGenLat int, minGenLon int, maxGenLat int, maxGenLon int, generateEmptyFiles bool) {
	log.Info().Msg("Generating Offline Map")
	EnsureOfflineMapsDirectories()
	file, err := os.Open("./map.osm.pbf")
	check(errors.Wrap(err, "could not open map pbf file"))
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

	log.Info().Msg("Scanning Ways")
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
			lanes, _ := strconv.ParseUint(tags["lanes"], 10, 8)
			tmpWay := TmpWay{
				Nodes:            make([]TmpNode, len(way.Nodes)),
				Name:             tags["name"],
				Ref:              tags["ref"],
				Hazard:           tags["hazard"],
				MaxSpeed:         ParseMaxSpeed(tags["maxspeed"]),
				MaxSpeedAdvisory: ParseMaxSpeed(tags["maxspeed:advisory"]),
				Lanes:            uint8(lanes),
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

	log.Info().Msg("Finding Bounds")
	for _, area := range areas {
		if area.MinLat < float64(minGenLat)-OVERLAP_BOX_DEGREES || area.MinLon < float64(minGenLon)-OVERLAP_BOX_DEGREES || area.MaxLat > float64(maxGenLat)+OVERLAP_BOX_DEGREES || area.MaxLon > float64(maxGenLon)+OVERLAP_BOX_DEGREES {
			continue
		}

		haveWays := Overlapping(allMinLat, allMinLon, allMaxLat, allMaxLon, area.MinLat-OVERLAP_BOX_DEGREES, area.MinLon-OVERLAP_BOX_DEGREES, area.MaxLat+OVERLAP_BOX_DEGREES, area.MaxLon+OVERLAP_BOX_DEGREES)
		if !haveWays && !generateEmptyFiles {
			continue
		}

		arena := capnp.MultiSegment([][]byte{})
		msg, seg, err := capnp.NewMessage(arena)
		check(errors.Wrap(err, "could not create capnp arena for offline data"))
		rootOffline, err := NewRootOffline(seg)
		check(errors.Wrap(err, "could not create capnp offline root"))

		for _, way := range scannedWays {
			overlaps := Overlapping(way.MinLat, way.MinLon, way.MaxLat, way.MaxLon, area.MinLat-OVERLAP_BOX_DEGREES, area.MinLon-OVERLAP_BOX_DEGREES, area.MaxLat+OVERLAP_BOX_DEGREES, area.MaxLon+OVERLAP_BOX_DEGREES)
			if overlaps {
				area.Ways = append(area.Ways, way)
			}
		}

		log.Info().Msg("Writing Area")
		ways, err := rootOffline.NewWays(int32(len(area.Ways)))
		check(errors.Wrap(err, "could not create ways in offline data"))
		rootOffline.SetMinLat(area.MinLat)
		rootOffline.SetMinLon(area.MinLon)
		rootOffline.SetMaxLat(area.MaxLat)
		rootOffline.SetMaxLon(area.MaxLon)
    rootOffline.SetOverlap(OVERLAP_BOX_DEGREES)
		for i, way := range area.Ways {
			w := ways.At(i)
			w.SetMinLat(way.MinLat)
			w.SetMinLon(way.MinLon)
			w.SetMaxLat(way.MaxLat)
			w.SetMaxLon(way.MaxLon)
			err := w.SetName(way.Name)
			check(errors.Wrap(err, "could not set way name"))
			err = w.SetRef(way.Ref)
			check(errors.Wrap(err, "could not set way ref"))
			err = w.SetHazard(way.Hazard)
			check(errors.Wrap(err, "could not set way hazard"))
			w.SetMaxSpeed(way.MaxSpeed)
			w.SetAdvisorySpeed(way.MaxSpeedAdvisory)
			w.SetLanes(way.Lanes)
			nodes, err := w.NewNodes(int32(len(way.Nodes)))
			check(errors.Wrap(err, "could not create way nodes"))
			for j, node := range way.Nodes {
				n := nodes.At(j)
				n.SetLatitude(node.Latitude)
				n.SetLongitude(node.Longitude)
			}
		}

		data, err := msg.MarshalPacked()
		check(errors.Wrap(err, "could not marshal offline data"))
		err = CreateBoundsDir(area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
		check(errors.Wrap(err, "could not create directory for bounds file"))
		err = os.WriteFile(GenerateBoundsFileName(area.MinLat, area.MinLon, area.MaxLat, area.MaxLon), data, 0o644)
		check(errors.Wrap(err, "could not write offline data to file"))
	}
	f, err := os.Open(BOUNDS_DIR)
	check(errors.Wrap(err, "could not open bounds directory"))
	err = f.Sync()
	check(errors.Wrap(err, "could not fsync bounds directory"))
	err = f.Close()
	check(errors.Wrap(err, "could not close bounds directory"))

	log.Info().Msg("Done Generating Offline Map")
}

func PointInBox(ax float64, ay float64, bxMin float64, byMin float64, bxMax float64, byMax float64) bool {
	return ax > bxMin && ax < bxMax && ay > byMin && ay < byMax
}

var AREAS = GenerateAreas()

func FindWaysAroundLocation(lat float64, lon float64) ([]byte, error) {
	for _, area := range AREAS {
		inBox := PointInBox(lat, lon, area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
		if inBox {
			boundsName := GenerateBoundsFileName(area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
			log.Info().Str("filename", boundsName).Msg("Loading bounds file")
			data, err := os.ReadFile(boundsName)
			return data, errors.Wrap(err, "could not read current offline data file")
		}
	}
	return []uint8{}, nil
}
