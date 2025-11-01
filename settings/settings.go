package settings

import (
	"encoding/json"
	"log/slog"
	"strings"

	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/params"
	"pfeifer.dev/mapd/utils"
	"time"
)

var (
	Settings = MapdSettings{}
)

type MapdSettings struct {
	VisionCurveSpeedControlEnabled bool    `json:"vision_curve_speed_control_enabled"`
	CurveSpeedControlEnabled       bool    `json:"curve_speed_control_enabled"`
	SpeedLimitControlEnabled       bool    `json:"speed_limit_control_enabled"`
	VtscUseEnableSpeed             bool    `json:"vtc_use_enable_speed"`
	SpeedLimitUseEnableSpeed       bool    `json:"speed_limit_use_enable_speed"`
	CurveUseEnableSpeed            bool    `json:"curve_use_enable_speed"`
	LogLevel                       string  `json:"log_level"`
	VtscTargetLatA                 float32 `json:"vtsc_target_lat_a"`
	VtscMinTargetV                 float32 `json:"vtsc_min_target_v"`
	SpeedLimitOffset               float32 `json:"speed_limit_offset"`
	EnableSpeed                    float32 `json:"enable_speed"`
}

func (s *MapdSettings) Default() {
	s.VtscMinTargetV = 5
	s.VtscTargetLatA = 1.9
	s.SpeedLimitOffset = 0
	s.LogLevel = "error"
	s.VisionCurveSpeedControlEnabled = false
	s.CurveSpeedControlEnabled = false
	s.SpeedLimitControlEnabled = false
	s.EnableSpeed = 0
	s.VtscUseEnableSpeed = false
	s.CurveUseEnableSpeed = false
	s.SpeedLimitUseEnableSpeed = false
}

func (s *MapdSettings) Load() (success bool) {
	data, err := params.GetParam(params.MAPD_SETTINGS)
	if err != nil {
		utils.Loge(err)
		return false
	}

	err = json.Unmarshal(data, s)
	if err != nil {
		utils.Loge(err)
		return false
	}

	s.setLogLevel()

	return true
}

func (s *MapdSettings) LoadWithFallback() {
	settingsLoaded := false
	for range 15 {
		if s.Load() {
			settingsLoaded = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !settingsLoaded {
		s.Default()
		s.Save()
	}
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
	case custom.MapdInputType_saveSettings:
		go s.Save()
	case custom.MapdInputType_setVtscMinTargetV:
		s.VtscMinTargetV = input.Float()
	case custom.MapdInputType_setVtscTargetLatA:
		s.VtscTargetLatA = input.Float()
	case custom.MapdInputType_setVisionCurveSpeedControl:
		s.VisionCurveSpeedControlEnabled = input.Bool()
	case custom.MapdInputType_setSpeedLimitControl:
		s.SpeedLimitControlEnabled = input.Bool()
	case custom.MapdInputType_setCurveSpeedControl:
		s.CurveSpeedControlEnabled = input.Bool()
	case custom.MapdInputType_setSpeedLimitOffset:
		s.SpeedLimitOffset = input.Float()
	case custom.MapdInputType_setEnableSpeed:
		s.EnableSpeed = input.Float()
	case custom.MapdInputType_setVtscUseEnableSpeed:
		s.VtscUseEnableSpeed = input.Bool()
	case custom.MapdInputType_setCurveUseEnableSpeed:
		s.CurveUseEnableSpeed = input.Bool()
	case custom.MapdInputType_setSpeedLimitUseEnableSpeed:
		s.SpeedLimitUseEnableSpeed = input.Bool()
	case custom.MapdInputType_setLogLevel:
		logLevel, err := input.Str()
		if err != nil {
			utils.Loge(err)
			return
		}
		s.LogLevel = logLevel
		s.setLogLevel()
	}
}
