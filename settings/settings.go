package settings

import (
	"encoding/json"
	"log/slog"
	"strings"

	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/params"
	"pfeifer.dev/mapd/utils"
)

var (
	Settings = MapdSettings{}
	loaded   = false
)

type MapdSettings struct {
	VtscTargetLatA                 float32 `json:"vtsc_target_lat_a"`
	VtscMinTargetV                 float32 `json:"vtsc_min_target_v"`
	SpeedLimitOffset               float32 `json:"speed_limit_offset"`
	LogLevel                       string  `json:"log_level"`
	VisionCurveSpeedControlEnabled bool    `json:"vision_curve_speed_control_enabled"`
	CurveSpeedControlEnabled       bool    `json:"curve_speed_control_enabled"`
	SpeedLimitControlEnabled       bool    `json:"speed_limit_control_enabled"`
}

func (s *MapdSettings) Default() {
	s.VtscMinTargetV = 5
	s.VtscTargetLatA = 1.9
	s.SpeedLimitOffset = 0
	s.LogLevel = "error"
	s.VisionCurveSpeedControlEnabled = false
	s.CurveSpeedControlEnabled = false
	s.SpeedLimitControlEnabled = false
}

func (s *MapdSettings) Load() {
	data, err := params.GetParam(params.MAPD_SETTINGS)
	if err != nil {
		if !loaded {
			s.Default()
			s.Save()
		} else {
			utils.Loge(err)
			return
		}
	}

	err = json.Unmarshal(data, s)
	if err != nil {
		if !loaded {
			s.Default()
			s.Save()
		} else {
			utils.Loge(err)
			return
		}
	}

	s.setLogLevel()

	loaded = true
}

func (s *MapdSettings) Save() {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		utils.Loge(err)
		return
	}
	err = params.PutParam(params.MAPD_SETTINGS, data)
	if err != nil {
		utils.Loge(err)
		return
	}
}

func (s *MapdSettings) setLogLevel() {
	switch strings.ToLower(s.LogLevel) {
	case "debug":
		slog.SetLogLoggerLevel(slog.LevelDebug)
	case "info":
		slog.SetLogLoggerLevel(slog.LevelInfo)
	case "warn":
		slog.SetLogLoggerLevel(slog.LevelWarn)
	case "error":
		slog.SetLogLoggerLevel(slog.LevelError)
	default:
		slog.SetLogLoggerLevel(slog.LevelError)
	}
}

func (s *MapdSettings) Handle(input custom.MapdIn) {
	switch input.Type() {
	case custom.MapdInputType_reloadSettings:
		s.Load()
	case custom.MapdInputType_setVtscMinTargetV:
		s.VtscMinTargetV = input.Float()
		s.Save()
	case custom.MapdInputType_setVtscTargetLatA:
		s.VtscTargetLatA = input.Float()
		s.Save()
	case custom.MapdInputType_setVisionCurveSpeedControl:
		s.VisionCurveSpeedControlEnabled = input.Bool()
		s.Save()
	case custom.MapdInputType_setSpeedLimitControl:
		s.SpeedLimitControlEnabled = input.Bool()
		s.Save()
	case custom.MapdInputType_setCurveSpeedControl:
		s.CurveSpeedControlEnabled = input.Bool()
		s.Save()
	case custom.MapdInputType_setSpeedLimitOffset:
		s.SpeedLimitOffset = input.Float()
		s.Save()
	case custom.MapdInputType_setLogLevel:
		s.LogLevel = input.String()
		s.setLogLevel()
		s.Save()
	}
}
