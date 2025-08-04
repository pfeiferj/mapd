package main

import (
	"encoding/json"
	"flag"
	"os"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

type State struct {
	Data       []uint8
	CurrentWay CurrentWay
	NextWays   []NextWayResult
	Position   Position
}

type Position struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Bearing   float64 `json:"bearing"`
}

type NextSpeedLimit struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Speedlimit float64 `json:"speedlimit"`
}

type AdvisoryLimit struct {
	StartLatitude  float64 `json:"start_latitude"`
	StartLongitude float64 `json:"start_longitude"`
	EndLatitude    float64 `json:"end_latitude"`
	EndLongitude   float64 `json:"end_longitude"`
	Speedlimit     float64 `json:"speedlimit"`
}

type Hazard struct {
	StartLatitude  float64 `json:"start_latitude"`
	StartLongitude float64 `json:"start_longitude"`
	EndLatitude    float64 `json:"end_latitude"`
	EndLongitude   float64 `json:"end_longitude"`
	Hazard         string  `json:"hazard"`
}

func RoadName(way Way) string {
	name, err := way.Name()
	if err == nil {
		if len(name) > 0 {
			return name
		}
	}
	ref, err := way.Ref()
	if err == nil {
		if len(ref) > 0 {
			return ref
		}
	}
	return ""
}

func readOffline(data []uint8) Offline {
	msg, err := capnp.UnmarshalPacked(data)
	logde(errors.Wrap(err, "could not unmarshal offline data"))
	if err == nil {
		offline, err := ReadRootOffline(msg)
		logde(errors.Wrap(err, "could not read offline message"))
		return offline
	}
	return Offline{}
}

func readPosition(persistent bool) (Position, error) {
	path := LAST_GPS_POSITION
	if persistent {
		path = LAST_GPS_POSITION_PERSIST
	}

	pos := Position{}
	coordinates, err := GetParam(path)
	if err != nil {
		return pos, errors.Wrap(err, "could not read coordinates param")
	}
	err = json.Unmarshal(coordinates, &pos)
	return pos, errors.Wrap(err, "could not unmarshal coordinates")
}

func getEffectiveMaxSpeed(way Way, isForward bool) float64 {
	if isForward && way.MaxSpeedForward() > 0 {
		return way.MaxSpeedForward()
	} else if !isForward && way.MaxSpeedBackward() > 0 {
		return way.MaxSpeedBackward()
	}
	return way.MaxSpeed()
}

func loop(state *State) {
	defer func() {
		if err := recover(); err != nil {
			e := errors.Errorf("panic occured: %v", err)
			loge(e)
			// reset state for next loop
			state.Data = []uint8{}
			state.NextWays = []NextWayResult{}
			state.CurrentWay = CurrentWay{}
			state.Position = Position{}
		}
	}()

	logLevelData, err := GetParam(MAPD_LOG_LEVEL)
	if err == nil {
		level, err := zerolog.ParseLevel(string(logLevelData))
		if err == nil {
			zerolog.SetGlobalLevel(level)
		}
	}
	prettyLog, err := GetParam(MAPD_PRETTY_LOG)
	if err == nil && len(prettyLog) > 0 && prettyLog[0] == '1' {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		logde(RemoveParam(MAPD_PRETTY_LOG))
	} else if err == nil && len(prettyLog) > 0 && prettyLog[0] == '0' {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
		logde(RemoveParam(MAPD_PRETTY_LOG))
	}

	target_lat_a, err := GetParam(MAP_TARGET_LAT_A)
	if err == nil && len(target_lat_a) > 0 {
		var t_lat_a float64
		err = json.Unmarshal(target_lat_a, &t_lat_a)
		if err == nil {
			TARGET_LAT_ACCEL = t_lat_a
			_ = RemoveParam(MAP_TARGET_LAT_A)
			log.Info().Float64("target_lat_accel", t_lat_a).Msg("loaded memory target lateral accel")
		}
	}

	time.Sleep(1 * time.Second)
	DownloadIfTriggered()

	pos, err := readPosition(false)
	if err != nil {
		logwe(errors.Wrap(err, "could not read current position"))
		return
	}
	offline := readOffline(state.Data)

	// ------------- Find current and next ways ------------

	if !PointInBox(pos.Latitude, pos.Longitude, offline.MinLat(), offline.MinLon(), offline.MaxLat(), offline.MaxLon()) {
		state.Data, err = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
		logde(errors.Wrap(err, "could not find ways around current location"))
	}

	state.CurrentWay, err = GetCurrentWay(state.CurrentWay, state.NextWays, offline, pos)
	logde(errors.Wrap(err, "could not get current way"))

	state.NextWays, err = NextWays(pos, state.CurrentWay, offline, state.CurrentWay.OnWay.IsForward)
	logde(errors.Wrap(err, "could not get next way"))

	curvatures, err := GetStateCurvatures(state)
	logde(errors.Wrap(err, "could not get curvatures from current state"))
	target_velocities := GetTargetVelocities(curvatures)

	// -----------------  Write data ---------------------

	// -----------------  MTSC Data  -----------------------
	data, err := json.Marshal(curvatures)
	logde(errors.Wrap(err, "could not marshal curvatures"))
	err = PutParam(MAP_CURVATURES, data)
	logwe(errors.Wrap(err, "could not write curvatures"))

	data, err = json.Marshal(target_velocities)
	logde(errors.Wrap(err, "could not marshal target velocities"))
	err = PutParam(MAP_TARGET_VELOCITIES, data)
	logwe(errors.Wrap(err, "could not write curvatures"))

	// ----------------- Current Data --------------------
	err = PutParam(ROAD_NAME, []byte(RoadName(state.CurrentWay.Way)))
	logwe(errors.Wrap(err, "could not write road name"))

	data, err = json.Marshal(state.CurrentWay.Way.AdvisorySpeed())
	logde(errors.Wrap(err, "could not marshal advisory speed limit"))
	err = PutParam(MAP_ADVISORY_LIMIT, data)
	logwe(errors.Wrap(err, "could not write advisory speed limit"))

	hazard, err := state.CurrentWay.Way.Hazard()
	logde(errors.Wrap(err, "could not read current way hazard"))
	data, err = json.Marshal(Hazard{
		StartLatitude:  state.CurrentWay.StartPosition.Latitude(),
		StartLongitude: state.CurrentWay.StartPosition.Longitude(),
		EndLatitude:    state.CurrentWay.EndPosition.Latitude(),
		EndLongitude:   state.CurrentWay.EndPosition.Longitude(),
		Hazard:         hazard,
	})
	logde(errors.Wrap(err, "could not marshal hazard"))
	err = PutParam(MAP_HAZARD, data)
	logwe(errors.Wrap(err, "could not write hazard"))

	data, err = json.Marshal(AdvisoryLimit{
		StartLatitude:  state.CurrentWay.StartPosition.Latitude(),
		StartLongitude: state.CurrentWay.StartPosition.Longitude(),
		EndLatitude:    state.CurrentWay.EndPosition.Latitude(),
		EndLongitude:   state.CurrentWay.EndPosition.Longitude(),
		Speedlimit:     state.CurrentWay.Way.AdvisorySpeed(),
	})
	logde(errors.Wrap(err, "could not marshal advisory speed limit"))
	err = PutParam(MAP_ADVISORY_LIMIT, data)
	logwe(errors.Wrap(err, "could not write advisory speed limit"))

	// ---------------- Next Data ---------------------

	if len(state.NextWays) > 0 {
		hazard, err = state.NextWays[0].Way.Hazard()
		logde(errors.Wrap(err, "could not read next hazard"))
		data, err = json.Marshal(Hazard{
			StartLatitude:  state.NextWays[0].StartPosition.Latitude(),
			StartLongitude: state.NextWays[0].StartPosition.Longitude(),
			EndLatitude:    state.NextWays[0].EndPosition.Latitude(),
			EndLongitude:   state.NextWays[0].EndPosition.Longitude(),
			Hazard:         hazard,
		})
		logde(errors.Wrap(err, "could not marshal next hazard"))
		err = PutParam(NEXT_MAP_HAZARD, data)
		logwe(errors.Wrap(err, "could not write next hazard"))
	}

	if len(state.NextWays) > 0 {
		currentMaxSpeed := state.CurrentWay.Way.MaxSpeed()
		if state.CurrentWay.OnWay.IsForward && state.CurrentWay.Way.MaxSpeedForward() > 0 {
			currentMaxSpeed = state.CurrentWay.Way.MaxSpeedForward()
		} else if !state.CurrentWay.OnWay.IsForward && state.CurrentWay.Way.MaxSpeedBackward() > 0 {
			currentMaxSpeed = state.CurrentWay.Way.MaxSpeedBackward()
		}

		data, err = json.Marshal(currentMaxSpeed)
		logde(errors.Wrap(err, "could not marshal speed limit"))
		err = PutParam(MAP_SPEED_LIMIT, data)
		logwe(errors.Wrap(err, "could not write speed limit"))

		nextMaxSpeed := currentMaxSpeed
		nextSpeedWay := state.NextWays[0]
		for _, nextWay := range state.NextWays {
			nextMaxSpeed = getEffectiveMaxSpeed(nextWay.Way, nextWay.IsForward)
			if nextMaxSpeed != currentMaxSpeed {
				nextSpeedWay = nextWay
				break
			}
		}
		data, err = json.Marshal(NextSpeedLimit{
			Latitude:   nextSpeedWay.StartPosition.Latitude(),
			Longitude:  nextSpeedWay.StartPosition.Longitude(),
			Speedlimit: nextMaxSpeed, // Use the calculated speed
		})
		logde(errors.Wrap(err, "could not marshal next speed limit"))
		err = PutParam(NEXT_MAP_SPEED_LIMIT, data)
		logwe(errors.Wrap(err, "could not write next speed limit"))
	} else {
		data, err = json.Marshal(NextSpeedLimit{})
		logde(errors.Wrap(err, "could not marshal next speed limit"))
		err = PutParam(NEXT_MAP_SPEED_LIMIT, data)
		logwe(errors.Wrap(err, "could not write next speed limit"))
	}

	if len(state.NextWays) > 0 {
		currentAdvisorySpeed := state.CurrentWay.Way.AdvisorySpeed()
		nextAdvisorySpeed := currentAdvisorySpeed
		nextAdvisoryWay := state.NextWays[0]

		for _, nextWay := range state.NextWays {
			nextAdvisorySpeed = nextWay.Way.AdvisorySpeed()
			if nextAdvisorySpeed != currentAdvisorySpeed {
				nextAdvisoryWay = nextWay
				break
			}
		}
		data, err = json.Marshal(AdvisoryLimit{
			StartLatitude:  nextAdvisoryWay.StartPosition.Latitude(),
			StartLongitude: nextAdvisoryWay.StartPosition.Longitude(),
			EndLatitude:    nextAdvisoryWay.EndPosition.Latitude(),
			EndLongitude:   nextAdvisoryWay.EndPosition.Longitude(),
			Speedlimit:     nextAdvisoryWay.Way.AdvisorySpeed(),
		})
		logde(errors.Wrap(err, "could not marshal next advisory speed limit"))
		err = PutParam(NEXT_MAP_ADVISORY_LIMIT, data)
		logwe(errors.Wrap(err, "could not write next advisory speed limit"))
	} else {
		data, err = json.Marshal(AdvisoryLimit{})
		logde(errors.Wrap(err, "could not marshal next advisory speed limit"))
		err = PutParam(NEXT_MAP_ADVISORY_LIMIT, data)
		logwe(errors.Wrap(err, "could not write next advisory speed limit"))
	}
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixNano
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	l := zerolog.InfoLevel
	logLevelData, err := GetParam(MAPD_LOG_LEVEL_PERSIST)
	if err == nil {
		level, err := zerolog.ParseLevel(string(logLevelData))
		if err == nil {
			l = level
		}
	}
	zerolog.SetGlobalLevel(l)
	prettyLog, err := GetParam(MAPD_PRETTY_LOG_PERSIST)
	if err == nil && len(prettyLog) > 0 && prettyLog[0] == '1' {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	generatePtr := flag.Bool("generate", false, "Triggers a generation of map data from 'map.osm.pbf'")
	minGenLatPtr := flag.Int("minlat", -90, "the minimum latitude to generate")
	minGenLonPtr := flag.Int("minlon", -180, "the minimum longitude to generate")
	maxGenLatPtr := flag.Int("maxlat", -90, "the maximum latitude to generate")
	maxGenLonPtr := flag.Int("maxlon", -180, "the maximum longitude to generate")
	generateEmptyFiles := flag.Bool("generate-empty-files", false, "Includes empty files when generating map")
	flag.Parse()
	if *generatePtr {
		GenerateOffline(*minGenLatPtr, *minGenLonPtr, *maxGenLatPtr, *maxGenLonPtr, *generateEmptyFiles)
		return
	}
	EnsureParamDirectories()
	ResetParams()
	state := State{}

	pos, err := readPosition(true)
	logde(err)
	if err == nil {
		state.Data, err = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
		logde(errors.Wrap(err, "could not find ways around initial location"))
	}

	target_lat_a, err := GetParam(MAP_TARGET_LAT_A_PERSIST)
	if err == nil && len(target_lat_a) > 0 {
		var t_lat_a float64
		err = json.Unmarshal(target_lat_a, &t_lat_a)
		if err == nil {
			TARGET_LAT_ACCEL = t_lat_a
			log.Info().Float64("target_lat_accel", t_lat_a).Msg("loaded persistent target lateral accel")
		}
	}

	for {
		loop(&state)
	}
}
