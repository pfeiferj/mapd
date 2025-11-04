package maps

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"

	"capnproto.org/go/capnp/v3"
	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal/offline"
	"pfeifer.dev/mapd/utils"
	"pfeifer.dev/mapd/params"
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
	MaxSpeedForward  float64
	MaxSpeedBackward float64
	MaxSpeedAdvisory float64
	Lanes            uint8
	MinLat           float64
	MinLon           float64
	MaxLat           float64
	MaxLon           float64
	OneWay           bool
	Nodes            []TmpNode
}

type Area struct {
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
	Ways   []TmpWay
}

type OfflineSettings struct {
	MinLat float64
	MinLon float64
	MaxLat float64
	MaxLon float64
	OutputDirectory string
	InputFile string
	GenerateEmptyFiles bool
	Overlap float64
}

var (
	GROUP_AREA_BOX_DEGREES = 2
	AREA_BOX_DEGREES       = float64(1.0 / 4) // Must be 1.0 divided by an integer number
	WAYS_PER_FILE          = 2000
	DEFAULT_SETTINGS       = OfflineSettings{
		OutputDirectory: fmt.Sprintf("%s/offline", params.GetBaseOpPath()),
	}
)

func EnsureOfflineMapsDirectories(s OfflineSettings) {
	err := os.MkdirAll(s.OutputDirectory, 0o775)
	utils.Logwe(err)
}

// Creates a file for a specific bounding box
func GenerateBoundsFileName(a Area, s OfflineSettings) string {
	group_lat_directory := int(math.Floor(a.MinLat/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	group_lon_directory := int(math.Floor(a.MinLon/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	dir := fmt.Sprintf("%s/%d/%d", s.OutputDirectory, group_lat_directory, group_lon_directory)
	return fmt.Sprintf("%s/%f_%f_%f_%f", dir, a.MinLat, a.MinLon, a.MaxLat, a.MaxLon)
}

// Creates a file for a specific bounding box
func CreateBoundsDir(a Area, s OfflineSettings) error {
	group_lat_directory := int(math.Floor(a.MinLat/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	group_lon_directory := int(math.Floor(a.MinLon/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	dir := fmt.Sprintf("%s/%d/%d", s.OutputDirectory, group_lat_directory, group_lon_directory)
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
func generateAreas() []Area {
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

func GenerateOffline(s OfflineSettings) {
	slog.Info("Generating Offline Map")
	EnsureOfflineMapsDirectories(s)
	file, err := os.Open("./map.osm.pbf")
	utils.Check(errors.Wrap(err, "could not open map pbf file"))
	defer file.Close()

	// The third parameter is the number of parallel decoders to use.
	scanner := osmpbf.New(context.Background(), file, runtime.GOMAXPROCS(-1))
	scanner.SkipRelations = true
	defer scanner.Close()

	scannedWays := []TmpWay{}
	areas := generateAreas()
	index := 0
	allMinLat := float64(90)
	allMinLon := float64(180)
	allMaxLat := float64(-90)
	allMaxLon := float64(-180)

	slog.Info("Scanning Ways")
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
				MaxSpeedForward:  ParseMaxSpeed(tags["maxspeed:forward"]),
				MaxSpeedBackward: ParseMaxSpeed(tags["maxspeed:backward"]),
				MaxSpeedAdvisory: ParseMaxSpeed(tags["maxspeed:advisory"]),
				Lanes:            uint8(lanes),
				OneWay:           tags["oneway"] == "yes",
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

	slog.Info("Finding Bounds")
	for _, area := range areas {
		if area.MinLat < s.MinLat || area.MinLon < s.MinLon || area.MaxLat > s.MaxLat || area.MaxLon > s.MaxLon {
			continue
		}

		haveWays := Overlapping(s.MinLat, s.MinLon, s.MaxLat, s.MaxLon, area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
		if !haveWays && !s.GenerateEmptyFiles {
			continue
		}

		arena := capnp.MultiSegment([][]byte{})
		msg, seg, err := capnp.NewMessage(arena)
		utils.Check(errors.Wrap(err, "could not create capnp arena for offline data"))
		rootOffline, err := offline.NewRootOffline(seg)
		utils.Check(errors.Wrap(err, "could not create capnp offline root"))

		for _, way := range scannedWays {
			overlaps := Overlapping(way.MinLat, way.MinLon, way.MaxLat, way.MaxLon, area.MinLat-s.Overlap, area.MinLon-s.Overlap, area.MaxLat+s.Overlap, area.MaxLon+s.Overlap)
			if overlaps {
				area.Ways = append(area.Ways, way)
			}
		}

		slog.Info("Writing Area")
		ways, err := rootOffline.NewWays(int32(len(area.Ways)))
		utils.Check(errors.Wrap(err, "could not create ways in offline data"))
		rootOffline.SetMinLat(area.MinLat)
		rootOffline.SetMinLon(area.MinLon)
		rootOffline.SetMaxLat(area.MaxLat)
		rootOffline.SetMaxLon(area.MaxLon)
		rootOffline.SetOverlap(s.Overlap)
		for i, way := range area.Ways {
			w := ways.At(i)
			w.SetMinLat(way.MinLat)
			w.SetMinLon(way.MinLon)
			w.SetMaxLat(way.MaxLat)
			w.SetMaxLon(way.MaxLon)
			err := w.SetName(way.Name)
			utils.Check(errors.Wrap(err, "could not set way name"))
			err = w.SetRef(way.Ref)
			utils.Check(errors.Wrap(err, "could not set way ref"))
			err = w.SetHazard(way.Hazard)
			utils.Check(errors.Wrap(err, "could not set way hazard"))
			w.SetMaxSpeed(way.MaxSpeed)
			w.SetMaxSpeedForward(way.MaxSpeedForward)
			w.SetMaxSpeedBackward(way.MaxSpeedBackward)
			w.SetAdvisorySpeed(way.MaxSpeedAdvisory)
			w.SetLanes(way.Lanes)
			w.SetOneWay(way.OneWay)
			nodes, err := w.NewNodes(int32(len(way.Nodes)))
			utils.Check(errors.Wrap(err, "could not create way nodes"))
			for j, node := range way.Nodes {
				n := nodes.At(j)
				n.SetLatitude(node.Latitude)
				n.SetLongitude(node.Longitude)
			}
		}

		data, err := msg.MarshalPacked()
		utils.Check(errors.Wrap(err, "could not marshal offline data"))
		err = CreateBoundsDir(area, s)
		utils.Check(errors.Wrap(err, "could not create directory for bounds file"))
		err = os.WriteFile(GenerateBoundsFileName(area, s), data, 0o644)
		utils.Check(errors.Wrap(err, "could not write offline data to file"))
	}
	f, err := os.Open(s.OutputDirectory)
	utils.Check(errors.Wrap(err, "could not open bounds directory"))
	err = f.Sync()
	utils.Check(errors.Wrap(err, "could not fsync bounds directory"))
	err = f.Close()
	utils.Check(errors.Wrap(err, "could not close bounds directory"))

	slog.Info("Done Generating Offline Map")
}

func PointInBox(ax float64, ay float64, bxMin float64, byMin float64, bxMax float64, byMax float64) bool {
	return ax > bxMin && ax < bxMax && ay > byMin && ay < byMax
}

var AREAS = generateAreas()

func FindWaysAroundLocation(lat float64, lon float64) ([]byte, error) {
	for _, area := range AREAS {
		inBox := PointInBox(lat, lon, area.MinLat, area.MinLon, area.MaxLat, area.MaxLon)
		if inBox {
			boundsName := GenerateBoundsFileName(area, DEFAULT_SETTINGS)
			slog.Info("Loading bounds file", "filename", boundsName)
			data, err := os.ReadFile(boundsName)
			return data, errors.Wrap(err, "could not read current offline data file")
		}
	}
	return []uint8{}, nil
}

func ParseMaxSpeed(maxspeed string) float64 {
	splitSpeed := strings.Split(maxspeed, " ")
	if len(splitSpeed) == 0 {
		return 0
	}

	numeric, err := strconv.ParseUint(splitSpeed[0], 10, 64)
	if err != nil {
		return 0
	}

	if len(splitSpeed) == 1 {
		return 0.277778 * float64(numeric)
	}

	if splitSpeed[1] == "kph" || splitSpeed[1] == "km/h" || splitSpeed[1] == "kmh" {
		return 0.277778 * float64(numeric)
	} else if splitSpeed[1] == "mph" {
		return 0.44704 * float64(numeric)
	} else if splitSpeed[1] == "knots" {
		return 0.514444 * float64(numeric)
	}

	return 0
}
