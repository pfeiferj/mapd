package maps

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"capnproto.org/go/capnp/v3"
	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
	"github.com/pkg/errors"
	"pfeifer.dev/mapd/cereal/offline"
	m "pfeifer.dev/mapd/math"
	"pfeifer.dev/mapd/params"
	ms "pfeifer.dev/mapd/settings"
	"pfeifer.dev/mapd/utils"
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
	Box              m.Box
	OneWay           bool
	HighwayClass     offline.HighwayClass
	Nodes            []TmpNode
	Id               int64

	MaxSpeedConditional         string
	MaxSpeedForwardConditional  string
	MaxSpeedBackwardConditional string
}

type Area struct {
	Box  m.Box
	Ways []TmpWay
}

type OfflineSettings struct {
	Box                m.Box
	OutputDirectory    string
	InputFile          string
	GenerateEmptyFiles bool
	Overlap            float64
}

var DEFAULT_SETTINGS = OfflineSettings{
	OutputDirectory: fmt.Sprintf("%s/offline", params.GetBaseOpPath()),
}

func EnsureOfflineMapsDirectories(s OfflineSettings) {
	if err := os.MkdirAll(s.OutputDirectory, 0o775); err != nil {
		slog.Warn("could not make offline maps directory", "error", err, "directory", s.OutputDirectory)
	}
}

func GenerateBoundsDirName(a Area, s OfflineSettings) string {
	p := a.Box.GroupPos()
	return fmt.Sprintf("%s/%d/%d", s.OutputDirectory, int(p.Lat()), int(p.Lon()))
}

// Creates a file for a specific bounding box
func GenerateBoundsFileName(a Area, s OfflineSettings) string {
	dir := GenerateBoundsDirName(a, s)
	return fmt.Sprintf("%s/%f_%f_%f_%f", dir, a.Box.MinPos.Lat(), a.Box.MinPos.Lon(), a.Box.MaxPos.Lat(), a.Box.MaxPos.Lon())
}

const (
	latitudeAreas  = int(180 / ms.AREA_BOX_DEGREES)
	longitudeAreas = int(360 / ms.AREA_BOX_DEGREES)
)

func areaBox(latitudeIndex int, longitudeIndex int) m.Box {
	minLatitude := -90 + float64(latitudeIndex)*ms.AREA_BOX_DEGREES
	minLongitude := -180 + float64(longitudeIndex)*ms.AREA_BOX_DEGREES
	return m.Box{
		MinPos: m.NewPosition(minLatitude, minLongitude),
		MaxPos: m.NewPosition(minLatitude+ms.AREA_BOX_DEGREES, minLongitude+ms.AREA_BOX_DEGREES),
	}
}

func areaIndexRange(minimum float64, maximum float64, origin float64, areaCount int) (int, int) {
	first := int(math.Floor((minimum-origin)/ms.AREA_BOX_DEGREES)) - 1
	last := int(math.Floor((maximum-origin)/ms.AREA_BOX_DEGREES)) + 1
	return max(first, 0), min(last, areaCount-1)
}

// Generates bounding boxes for storing ways
func generateAreas() []Area {
	areas := make([]Area, latitudeAreas*longitudeAreas)
	for i := range latitudeAreas {
		for j := range longitudeAreas {
			areas[i*longitudeAreas+j].Box = areaBox(i, j)
		}
	}
	return areas
}

func panicOnError(err error, message string) {
	if err != nil {
		slog.Error(message, "error", err)
		panic("failed to generate offline maps, exiting")
	}
}

func GenerateOffline(s OfflineSettings) {
	slog.Info("Generating Offline Map")
	EnsureOfflineMapsDirectories(s)
	file, err := os.Open(s.InputFile)
	panicOnError(err, "could not open map pbf file")
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
				Id:               int64(way.ID),
				HighwayClass:     HighwayClassFromTag(tags["highway"]),

				MaxSpeedConditional:         tags["maxspeed:conditional"],
				MaxSpeedForwardConditional:  tags["maxspeed:forward:conditional"],
				MaxSpeedBackwardConditional: tags["maxspeed:backward:conditional"],
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
			switch tags["oneway"] {
			case "yes", "1":
				tmpWay.OneWay = true
			case "-1":
				tmpWay.OneWay = true
				slices.Reverse(tmpWay.Nodes)
				tmpWay.MaxSpeedForward, tmpWay.MaxSpeedBackward = tmpWay.MaxSpeedBackward, tmpWay.MaxSpeedForward
				tmpWay.MaxSpeedForwardConditional, tmpWay.MaxSpeedBackwardConditional = tmpWay.MaxSpeedBackwardConditional, tmpWay.MaxSpeedForwardConditional
			case "":
				tmpWay.OneWay = tags["junction"] == "roundabout" || tags["highway"] == "motorway"
			}
			tmpWay.Box.MinPos = m.NewPosition(minLat, minLon)
			tmpWay.Box.MaxPos = m.NewPosition(maxLat, maxLon)
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
	panicOnError(scanner.Err(), "could not scan map pbf file")

	slog.Info("Finding Bounds")
	overlap := math.Abs(s.Overlap)
	areaWays := make([][]int, len(areas))

	for wayIndex, way := range scannedWays {
		firstLatitude, lastLatitude := areaIndexRange(way.Box.MinPos.Lat()-overlap, way.Box.MaxPos.Lat()+overlap, -90, latitudeAreas)
		firstLongitude, lastLongitude := areaIndexRange(way.Box.MinPos.Lon()-overlap, way.Box.MaxPos.Lon()+overlap, -180, longitudeAreas)

		for i := firstLatitude; i <= lastLatitude; i++ {
			for j := firstLongitude; j <= lastLongitude; j++ {
				areaIndex := i*longitudeAreas + j

				if way.Box.Overlapping(areas[areaIndex].Box.Overlap(s.Overlap)) {
					areaWays[areaIndex] = append(areaWays[areaIndex], wayIndex)
				}
			}
		}
	}

	for areaIndex, area := range areas {
		overlapBox := s.Box.Overlap(s.Overlap)
		if !overlapBox.Contains(area.Box) {
			continue
		}

		haveWays := overlapBox.Overlapping(area.Box)
		if !haveWays && !s.GenerateEmptyFiles {
			continue
		}

		arena := capnp.MultiSegment(nil)
		msg, seg, err := capnp.NewMessage(arena)
		panicOnError(err, "could not create capnp arena for offline data")
		rootOffline, err := offline.NewRootOffline(seg)
		panicOnError(err, "could not create capnp root for offline data")

		for _, wayIndex := range areaWays[areaIndex] {
			area.Ways = append(area.Ways, scannedWays[wayIndex])
		}

		slog.Info("Writing Area")
		ways, err := rootOffline.NewWays(int32(len(area.Ways)))
		panicOnError(err, "could not create ways in offline data")
		rootOffline.SetMinLat(area.Box.MinPos.Lat())
		rootOffline.SetMinLon(area.Box.MinPos.Lon())
		rootOffline.SetMaxLat(area.Box.MaxPos.Lat())
		rootOffline.SetMaxLon(area.Box.MaxPos.Lon())
		rootOffline.SetOverlap(s.Overlap)
		for i, way := range area.Ways {
			w := ways.At(i)
			w.SetId(way.Id)
			w.SetMinLat(way.Box.MinPos.Lat())
			w.SetMinLon(way.Box.MinPos.Lon())
			w.SetMaxLat(way.Box.MaxPos.Lat())
			w.SetMaxLon(way.Box.MaxPos.Lon())
			panicOnError(w.SetName(way.Name), "could not set way name")
			panicOnError(w.SetRef(way.Ref), "could not set way ref")
			panicOnError(w.SetHazard(way.Hazard), "could not set way hazard")
			w.SetMaxSpeed(way.MaxSpeed)
			w.SetMaxSpeedForward(way.MaxSpeedForward)
			w.SetMaxSpeedBackward(way.MaxSpeedBackward)
			panicOnError(w.SetMaxSpeedConditional(way.MaxSpeedConditional), "could not set way conditional max speed")
			panicOnError(w.SetMaxSpeedForwardConditional(way.MaxSpeedForwardConditional), "could not set way forward conditional max speed")
			panicOnError(w.SetMaxSpeedBackwardConditional(way.MaxSpeedBackwardConditional), "could not set way backward conditional max speed")
			w.SetAdvisorySpeed(way.MaxSpeedAdvisory)
			w.SetLanes(way.Lanes)
			w.SetOneWay(way.OneWay)
			w.SetHighwayClass(way.HighwayClass)
			nodes, err := w.NewNodes(int32(len(way.Nodes)))
			panicOnError(err, "could not create way nodes")
			for j, node := range way.Nodes {
				n := nodes.At(j)
				n.SetLatitude(node.Latitude)
				n.SetLongitude(node.Longitude)
			}
		}

		data, err := msg.MarshalPacked()
		panicOnError(err, "could not marshal offline data")
		panicOnError(os.MkdirAll(GenerateBoundsDirName(area, s), 0o775), "could not create bounds directory")
		panicOnError(os.WriteFile(GenerateBoundsFileName(area, s), data, 0o644), "could not write offline data to file")
	}
	f, err := os.Open(s.OutputDirectory)
	panicOnError(err, "could not open bounds directory")
	panicOnError(f.Sync(), "could not fsync bounds directory")
	panicOnError(f.Close(), "could not close bounds directory")

	slog.Info("Done Generating Offline Map")
}

func areaForPosition(pos m.Position) (Area, bool) {
	latitude := pos.Lat()
	longitude := pos.Lon()
	if math.IsNaN(latitude) || math.IsNaN(longitude) ||
		latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return Area{}, false
	}

	latitudeIndex := int(math.Ceil(latitude/ms.AREA_BOX_DEGREES)) + latitudeAreas/2 - 1
	longitudeIndex := int(math.Ceil(longitude/ms.AREA_BOX_DEGREES)) + longitudeAreas/2 - 1
	if latitudeIndex < 0 {
		latitudeIndex = 0
	}
	if longitudeIndex < 0 {
		longitudeIndex = 0
	}

	return Area{Box: areaBox(latitudeIndex, longitudeIndex)}, true
}

func FindWaysAroundPosition(pos m.Position) (Offline, error) {
	area, found := areaForPosition(pos)
	if !found {
		cBox := utils.Curry[m.Box]{}
		cBox.Set(area.Box)
		return Offline{Loaded: false, box: cBox}, nil
	}

	boundsName := GenerateBoundsFileName(area, DEFAULT_SETTINGS)
	slog.Info("Loading bounds file", "filename", boundsName)
	data, err := os.ReadFile(boundsName)
	o := ReadOffline(data)
	if !o.Loaded {
		o.box.Set(area.Box)
	}
	return o, errors.Wrap(err, "could not read current offline data file")
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
