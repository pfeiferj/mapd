package main

import (
	"encoding/json"
	"flag"
	"math"
	"os"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

type State struct {
	Data                   []uint8
	CurrentWay             CurrentWay
	NextWays               []NextWayResult
	Position               Position
	LastPosition           Position
	StableWayCounter       int
	LastGPSUpdate          time.Time
	LastWayChange          time.Time
	LastSpeedLimitDistance float64
	LastSpeedLimitValue    float64
	LastSpeedLimitWayName  string
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
	Distance   float64 `json:"distance"`
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

func shouldUpdateRoadInfo(state *State) bool {
	return true
}

func hasRoadInfoChanged(oldWay, newWay Way) bool {
	if !oldWay.HasNodes() && !newWay.HasNodes() {
		return false
	}
	if !oldWay.HasNodes() || !newWay.HasNodes() {
		return true
	}

	return !(oldWay.MinLat() == newWay.MinLat() &&
		oldWay.MaxLat() == newWay.MaxLat() &&
		oldWay.MinLon() == newWay.MinLon() &&
		oldWay.MaxLon() == newWay.MaxLon())
}

// Calculate distance from current position to a specific coordinate
func calculateDistanceToPoint(pos Position, lat float64, lon float64) float64 {
	return DistanceToPoint(pos.Latitude*TO_RADIANS, pos.Longitude*TO_RADIANS, lat*TO_RADIANS, lon*TO_RADIANS)
}

func calculateNextSpeedLimit(state *State, currentMaxSpeed float64) NextSpeedLimit {
	if len(state.NextWays) == 0 {
		return NextSpeedLimit{}
	}

	// Find the next speed limit change
	cumulativeDistance := 0.0
	currentPos := state.Position

	if state.CurrentWay.Way.HasNodes() {
		distToEnd, err := DistanceToEndOfWay(currentPos, state.CurrentWay.Way, state.CurrentWay.OnWay.IsForward)
		if err == nil && distToEnd > 0 {
			cumulativeDistance = distToEnd
		}
	}

	// Look through next ways for speed limit change
	for _, nextWay := range state.NextWays {
		nextMaxSpeed := nextWay.Way.MaxSpeed()
		if nextWay.IsForward && nextWay.Way.MaxSpeedForward() > 0 {
			nextMaxSpeed = nextWay.Way.MaxSpeedForward()
		} else if !nextWay.IsForward && nextWay.Way.MaxSpeedBackward() > 0 {
			nextMaxSpeed = nextWay.Way.MaxSpeedBackward()
		}

		if nextMaxSpeed != currentMaxSpeed && nextMaxSpeed > 0 {
			result := NextSpeedLimit{
				Latitude:   nextWay.StartPosition.Latitude(),
				Longitude:  nextWay.StartPosition.Longitude(),
				Speedlimit: nextMaxSpeed,
				Distance:   cumulativeDistance,
			}

			wayName := RoadName(nextWay.Way)
			if nextMaxSpeed == state.LastSpeedLimitValue && wayName == state.LastSpeedLimitWayName {
				smoothedDistance := state.LastSpeedLimitDistance*0.8 + cumulativeDistance*0.2
				if math.Abs(smoothedDistance-state.LastSpeedLimitDistance) < 50 {
					result.Distance = smoothedDistance
				}

				log.Debug().
					Float64("raw_distance", cumulativeDistance).
					Float64("smoothed_distance", result.Distance).
					Float64("last_distance", state.LastSpeedLimitDistance).
					Str("way", wayName).
					Msg("Smoothed speed limit distance")
			}
			state.LastSpeedLimitDistance = result.Distance
			state.LastSpeedLimitValue = nextMaxSpeed
			state.LastSpeedLimitWayName = wayName

			return result
		}
		if nextWay.Way.HasNodes() {
			wayDistance, err := calculateWayDistance(nextWay.Way)
			if err == nil {
				cumulativeDistance += wayDistance
			}
		}
	}

	return NextSpeedLimit{}
}

func calculateWayDistance(way Way) (float64, error) {
	nodes, err := way.Nodes()
	if err != nil {
		return 0, err
	}

	if nodes.Len() < 2 {
		return 0, nil
	}

	totalDistance := 0.0
	for i := 0; i < nodes.Len()-1; i++ {
		nodeStart := nodes.At(i)
		nodeEnd := nodes.At(i + 1)
		distance := DistanceToPoint(
			nodeStart.Latitude()*TO_RADIANS,
			nodeStart.Longitude()*TO_RADIANS,
			nodeEnd.Latitude()*TO_RADIANS,
			nodeEnd.Longitude()*TO_RADIANS,
		)
		totalDistance += distance
	}

	return totalDistance, nil
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
			state.LastPosition = Position{}
			state.StableWayCounter = 0
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

	state.LastPosition = state.Position

	pos, err := readPosition(false)
	if err != nil {
		logwe(errors.Wrap(err, "could not read current position"))
		return
	}

	state.Position = pos
	state.LastGPSUpdate = time.Now()

	offline := readOffline(state.Data)

	// ------------- Find current and next ways ------------

	if !PointInBox(pos.Latitude, pos.Longitude, offline.MinLat(), offline.MinLon(), offline.MaxLat(), offline.MaxLon()) {
		state.Data, err = FindWaysAroundLocation(pos.Latitude, pos.Longitude)
		logde(errors.Wrap(err, "could not find ways around current location"))
		state.CurrentWay.ConfidenceCounter = 0
	}

	oldWay := state.CurrentWay.Way

	// get GPS accuracy estimate
	gpsAccuracy := 5.0 // Default 5m accuracy

	state.CurrentWay, err = GetCurrentWay(state.CurrentWay, state.NextWays, offline, pos, state.LastPosition, gpsAccuracy)
	logde(errors.Wrap(err, "could not get current way"))

	if hasRoadInfoChanged(oldWay, state.CurrentWay.Way) {
		state.LastWayChange = time.Now()
		state.StableWayCounter = 0
		state.LastSpeedLimitDistance = 0
		state.LastSpeedLimitValue = 0
		state.LastSpeedLimitWayName = ""
		log.Info().
			Str("new_road", RoadName(state.CurrentWay.Way)).
			Float64("distance", state.CurrentWay.Distance.Distance).
			Int("confidence", state.CurrentWay.ConfidenceCounter).
			Msg("Way changed")
	} else {
		state.StableWayCounter++
	}

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

	if shouldUpdateRoadInfo(state) {
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

		if len(state.NextWays) > 0 || state.CurrentWay.Way.HasNodes() {
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

			nextSpeedLimit := calculateNextSpeedLimit(state, currentMaxSpeed)
			data, err = json.Marshal(nextSpeedLimit)
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
				if nextAdvisorySpeed == currentAdvisorySpeed {
					nextAdvisoryWay = nextWay
					nextAdvisorySpeed = nextWay.Way.AdvisorySpeed()
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

	// Log current status for debugging
	if state.CurrentWay.Way.HasNodes() {
		log.Debug().
			Str("road", RoadName(state.CurrentWay.Way)).
			Float64("distance", state.CurrentWay.Distance.Distance).
			Int("confidence", state.CurrentWay.ConfidenceCounter).
			Int("stable_counter", state.StableWayCounter).
			Float64("stable_distance", state.CurrentWay.StableDistance).
			Msg("Current way status")
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

	state := State{
		LastGPSUpdate: time.Now(),
		LastWayChange: time.Now(),
	}

	pos, err := readPosition(true)
	logde(err)
	if err == nil {
		state.Position = pos
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

	log.Info().Msg("Starting main loop with stability improvements and distance smoothing")
	for {
		loop(&state)
	}
}
