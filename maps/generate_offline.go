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

func (a *Area) OverlapBox(overlap float64) m.Box {
	return m.Box{
		MinPos: m.NewPosition(a.Box.MinPos.Lat()-overlap, a.Box.MinPos.Lon()-overlap),
		MaxPos: m.NewPosition(a.Box.MaxPos.Lat()+overlap, a.Box.MaxPos.Lon()+overlap),
	}
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

// Creates a file for a specific bounding box
func GenerateBoundsFileName(a Area, s OfflineSettings) string {
	p := a.Box.GroupPos()
	dir := fmt.Sprintf("%s/%d/%d", s.OutputDirectory, int(p.Lat()), int(p.Lon()))
	return fmt.Sprintf("%s/%f_%f_%f_%f", dir, a.Box.MinPos.Lat(), a.Box.MinPos.Lon(), a.Box.MaxPos.Lat(), a.Box.MaxPos.Lon())
}

// Creates a file for a specific bounding box
func CreateBoundsDir(a Area, s OfflineSettings) error {
	p := a.Box.GroupPos()
	dir := fmt.Sprintf("%s/%d/%d", s.OutputDirectory, int(p.Lat()), int(p.Lon()))
	err := os.MkdirAll(dir, 0o775)
	return errors.Wrap(err, "could not create bounds directory")
}

// Generates bounding boxes for storing ways
func generateAreas() []Area {
	areas := make([]Area, int((361/ms.AREA_BOX_DEGREES)*(181/ms.AREA_BOX_DEGREES)))
	index := 0
	for i := float64(-90); i < 90; i += ms.AREA_BOX_DEGREES {
		for j := float64(-180); j < 180; j += ms.AREA_BOX_DEGREES {
			a := &areas[index]
			a.Box = m.Box{
				MinPos: m.NewPosition(i, j),
				MaxPos: m.NewPosition(i+ms.AREA_BOX_DEGREES, j+ms.AREA_BOX_DEGREES),
			}
			index += 1
		}
	}
	return areas
}

func canBucketWay(way TmpWay) bool {
	return !math.IsNaN(way.Box.MinPos.Lat()) && !math.IsNaN(way.Box.MinPos.Lon()) &&
		!math.IsNaN(way.Box.MaxPos.Lat()) && !math.IsNaN(way.Box.MaxPos.Lon()) &&
		!math.IsInf(way.Box.MinPos.Lat(), 0) && !math.IsInf(way.Box.MinPos.Lon(), 0) &&
		!math.IsInf(way.Box.MaxPos.Lat(), 0) && !math.IsInf(way.Box.MaxPos.Lon(), 0) &&
		way.Box.MinPos.Lat() <= way.Box.MaxPos.Lat() && way.Box.MinPos.Lon() <= way.Box.MaxPos.Lon()
}

func bucketWaysByAreaGroup(scannedWays []TmpWay, areas []Area, s OfflineSettings) (map[m.Position][]int, []int, bool) {
	if s.Overlap < 0 || math.IsNaN(s.Overlap) || math.IsInf(s.Overlap, 0) {
		return nil, nil, false
	}
	for _, way := range scannedWays {
		if !canBucketWay(way) {
			return nil, nil, false
		}
	}

	selectedBounds := s.Box.Overlap(s.Overlap)
	groupBoxes := map[m.Position]m.Box{}
	for _, area := range areas {
		if !selectedBounds.Contains(area.Box) {
			continue
		}
		groupPos := area.Box.GroupPos()
		groupBox := m.Box{
			MinPos: groupPos,
			MaxPos: m.NewPosition(
				groupPos.Lat()+float64(ms.GROUP_AREA_BOX_DEGREES),
				groupPos.Lon()+float64(ms.GROUP_AREA_BOX_DEGREES),
			),
		}
		groupBoxes[groupPos] = groupBox.Overlap(s.Overlap)
	}

	groupWays := make(map[m.Position][]int, len(groupBoxes))
	sharedWays := []int{}
	for wayIndex, way := range scannedWays {
		var matchedGroup m.Position
		matched := false
		shared := false
		for groupPos, groupBox := range groupBoxes {
			if way.Box.Overlapping(groupBox) {
				if matched {
					shared = true
					break
				}
				matchedGroup = groupPos
				matched = true
			}
		}
		if shared {
			sharedWays = append(sharedWays, wayIndex)
		} else if matched {
			groupWays[matchedGroup] = append(groupWays[matchedGroup], wayIndex)
		}
	}
	return groupWays, sharedWays, true
}

func GenerateOffline(s OfflineSettings) {
	slog.Info("Generating Offline Map")
	EnsureOfflineMapsDirectories(s)
	file, err := os.Open("./map.osm.pbf")
	if err != nil {
		slog.Error("could not open map pbf file", "error", err)
		panic("failed to read maps, exiting")
	}
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

	overlapBox := s.Box.Overlap(s.Overlap)
	groupWays, sharedWays, bucketed := bucketWaysByAreaGroup(scannedWays, areas, s)
	slog.Info("Finding Bounds")
	for _, area := range areas {
		if !overlapBox.Contains(area.Box) {
			continue
		}

		haveWays := overlapBox.Overlapping(area.Box)
		if !haveWays && !s.GenerateEmptyFiles {
			continue
		}

		arena := capnp.MultiSegment(nil)
		msg, seg, err := capnp.NewMessage(arena)
		if err != nil {
			slog.Error("could not create capnp arena for offline data", "error", err)
			panic("unexpected capnp error, exiting")
		}
		rootOffline, err := offline.NewRootOffline(seg)
		if err != nil {
			slog.Error("could not create capnp root for offline data", "error", err)
			panic("unexpected capnp error, exiting")
		}

		areaOverlapBox := area.OverlapBox(s.Overlap)
		if bucketed {
			groupCandidates := groupWays[area.Box.GroupPos()]
			groupIndex := 0
			sharedIndex := 0
			for groupIndex < len(groupCandidates) || sharedIndex < len(sharedWays) {
				var wayIndex int
				if sharedIndex >= len(sharedWays) || (groupIndex < len(groupCandidates) && groupCandidates[groupIndex] < sharedWays[sharedIndex]) {
					wayIndex = groupCandidates[groupIndex]
					groupIndex++
				} else {
					wayIndex = sharedWays[sharedIndex]
					sharedIndex++
				}
				way := scannedWays[wayIndex]
				overlaps := way.Box.Overlapping(areaOverlapBox)
				if overlaps {
					area.Ways = append(area.Ways, way)
				}
			}
		} else {
			for _, way := range scannedWays {
				overlaps := way.Box.Overlapping(areaOverlapBox)
				if overlaps {
					area.Ways = append(area.Ways, way)
				}
			}
		}

		slog.Info("Writing Area")
		ways, err := rootOffline.NewWays(int32(len(area.Ways)))
		if err != nil {
			slog.Error("could not create ways in offline data", "error", err)
			panic("unexpected capnp error, exiting")
		}
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
			err := w.SetName(way.Name)
			if err != nil {
				slog.Error("could not set way name", "error", err)
				panic("unexpected capnp error, exiting")
			}
			err = w.SetRef(way.Ref)
			if err != nil {
				slog.Error("could not set way ref", "error", err)
				panic("unexpected capnp error, exiting")
			}
			err = w.SetHazard(way.Hazard)
			if err != nil {
				slog.Error("could not set way hazard", "error", err)
				panic("unexpected capnp error, exiting")
			}
			w.SetMaxSpeed(way.MaxSpeed)
			w.SetMaxSpeedForward(way.MaxSpeedForward)
			w.SetMaxSpeedBackward(way.MaxSpeedBackward)
			err = w.SetMaxSpeedConditional(way.MaxSpeedConditional)
			if err != nil {
				slog.Error("could not set way conditional max speed", "error", err)
				panic("unexpected capnp error, exiting")
			}
			err = w.SetMaxSpeedForwardConditional(way.MaxSpeedForwardConditional)
			if err != nil {
				slog.Error("could not set way forward conditional max speed", "error", err)
				panic("unexpected capnp error, exiting")
			}
			err = w.SetMaxSpeedBackwardConditional(way.MaxSpeedBackwardConditional)
			if err != nil {
				slog.Error("could not set way backward conditional max speed", "error", err)
				panic("unexpected capnp error, exiting")
			}
			w.SetAdvisorySpeed(way.MaxSpeedAdvisory)
			w.SetLanes(way.Lanes)
			w.SetOneWay(way.OneWay)
			w.SetHighwayClass(way.HighwayClass)
			nodes, err := w.NewNodes(int32(len(way.Nodes)))
			if err != nil {
				slog.Error("could not create way nodes", "error", err)
				panic("unexpected capnp error, exiting")
			}
			for j, node := range way.Nodes {
				n := nodes.At(j)
				n.SetLatitude(node.Latitude)
				n.SetLongitude(node.Longitude)
			}
		}

		data, err := msg.MarshalPacked()
		if err != nil {
			slog.Error("could not marshal offline data", "error", err)
			panic("unexpected capnp error, exiting")
		}
		err = CreateBoundsDir(area, s)
		if err != nil {
			slog.Error("could not create bounds directory", "error", err)
			panic("unexpected file error, exiting")
		}
		err = os.WriteFile(GenerateBoundsFileName(area, s), data, 0o644)
		if err != nil {
			slog.Error("could not write offline data to file", "error", err)
			panic("unexpected file error, exiting")
		}
	}
	f, err := os.Open(s.OutputDirectory)
	if err != nil {
		slog.Error("could not open bounds directory", "error", err)
		panic("unexpected file error, exiting")
	}
	err = f.Sync()
	if err != nil {
		slog.Error("could not fsync bounds directory", "error", err)
		panic("unexpected file error, exiting")
	}
	err = f.Close()
	if err != nil {
		slog.Error("could not close bounds directory", "error", err)
		panic("unexpected file error, exiting")
	}

	slog.Info("Done Generating Offline Map")
}

var AREAS = generateAreas()

func FindWaysAroundPosition(pos m.Position) (Offline, error) {
	for _, area := range AREAS {
		inBox := area.Box.PosInside(pos)
		if inBox {
			boundsName := GenerateBoundsFileName(area, DEFAULT_SETTINGS)
			slog.Info("Loading bounds file", "filename", boundsName)
			data, err := os.ReadFile(boundsName)
			o := ReadOffline(data)
			if !o.Loaded {
				area := Area{}
				areas := generateAreas()
				for _, a := range areas {
					if a.Box.PosInside(pos) {
						area = a
					}
				}
				o.box.Set(area.Box)
			}
			return o, errors.Wrap(err, "could not read current offline data file")
		}
	}
	area := Area{}
	areas := generateAreas()
	for _, a := range areas {
		if a.Box.PosInside(pos) {
			area = a
		}
	}
	cBox := utils.Curry[m.Box]{}
	cBox.Set(area.Box)
	return Offline{
		Loaded: false,
		box:    cBox,
	}, nil
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
