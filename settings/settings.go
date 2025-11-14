package settings

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/params"
)

var Settings = MapdSettings{
	downloadProgress: make(chan DownloadProgress, 1),
	cancelDownload: make(chan bool, 1),
}

type MapdSettings struct {
	downloadProgress                    chan DownloadProgress
	cancelDownload                      chan bool
	downloadActive                      bool
	VisionCurveSpeedControlEnabled      bool    `json:"vision_curve_speed_control_enabled"`
	CurveSpeedControlEnabled            bool    `json:"curve_speed_control_enabled"`
	SpeedLimitControlEnabled            bool    `json:"speed_limit_control_enabled"`
	VisionCurveUseEnableSpeed           bool    `json:"vision_curve_use_enable_speed"`
	SpeedLimitUseEnableSpeed            bool    `json:"speed_limit_use_enable_speed"`
	CurveUseEnableSpeed                 bool    `json:"curve_use_enable_speed"`
	LogLevel                            string  `json:"log_level"`
	VisionCurveTargetLatA               float32 `json:"vision_curve_target_lat_a"`
	VisionCurveMinTargetV               float32 `json:"vision_curve_min_target_v"`
	SpeedLimitOffset                    float32 `json:"speed_limit_offset"`
	EnableSpeed                         float32 `json:"enable_speed"`
	HoldLastSeenSpeedLimit              bool    `json:"hold_last_seen_speed_limit"`
	CurveTargetJerk                     float32 `json:"curve_target_jerk"`
	CurveTargetAccel                    float32 `json:"curve_target_accel"`
	CurveTargetOffset                   float32 `json:"curve_target_offset"`
	DefaultLaneWidth                    float32 `json:"default_lane_width"`
	CurveTargetLatA                     float32 `json:"curve_target_lat_a"`
	SlowDownForNextSpeedLimit           bool    `json:"slow_down_for_next_speed_limit"`
	SpeedUpForNextSpeedLimit            bool    `json:"speed_up_for_next_speed_limit"`
	HoldSpeedLimitWhileChangingSetSpeed bool    `json:"hold_speed_limit_while_changing_set_speed"`
}

func (s *MapdSettings) Default() {
	s.VisionCurveMinTargetV = 10 * MPH_TO_MS
	s.VisionCurveTargetLatA = 1.9
	s.SpeedLimitOffset = 0
	s.LogLevel = "error"
	s.VisionCurveSpeedControlEnabled = false
	s.CurveSpeedControlEnabled = false
	s.SpeedLimitControlEnabled = false
	s.EnableSpeed = 0
	s.VisionCurveUseEnableSpeed = false
	s.CurveUseEnableSpeed = false
	s.SpeedLimitUseEnableSpeed = false
	s.HoldLastSeenSpeedLimit = false
	s.CurveTargetJerk = -0.6
	s.CurveTargetAccel = -1.2
	s.CurveTargetOffset = 1.5
	s.DefaultLaneWidth = 3.7
	s.CurveTargetLatA = 2.0
	s.SpeedUpForNextSpeedLimit = false
	s.SlowDownForNextSpeedLimit = true
	s.HoldSpeedLimitWhileChangingSetSpeed = true
}

func (s *MapdSettings) Recommended() {
	s.VisionCurveMinTargetV = 10 * MPH_TO_MS
	s.VisionCurveTargetLatA = 1.9
	s.SpeedLimitOffset = 5 * MPH_TO_MS
	s.LogLevel = "error"
	s.VisionCurveSpeedControlEnabled = true
	s.CurveSpeedControlEnabled = true
	s.SpeedLimitControlEnabled = true
	s.EnableSpeed = 80 * MPH_TO_MS
	s.SpeedLimitUseEnableSpeed = true
	s.VisionCurveUseEnableSpeed = false
	s.CurveUseEnableSpeed = false
	s.HoldLastSeenSpeedLimit = true
	s.CurveTargetJerk = -0.6
	s.CurveTargetAccel = -1.2
	s.CurveTargetOffset = 1.5
	s.DefaultLaneWidth = 3.7
	s.CurveTargetLatA = 2.0
	s.SpeedUpForNextSpeedLimit = true
	s.SlowDownForNextSpeedLimit = true
	s.HoldSpeedLimitWhileChangingSetSpeed = true
}

func (s *MapdSettings) Load() (success bool) {
	s.Default() // set defaults so settings not already in param are defaulted
	data, err := params.GetParam(params.MAPD_SETTINGS)
	if err != nil {
		slog.Warn("failed to read MAPD_SETTINGS param", "error", err)
		return false
	}

	err = json.Unmarshal(data, s)
	if err != nil {
		slog.Warn("failed to parse MAPD_SETTINGS param", "error", err)
		return false
	}

	s.setLogLevel()

	return true
}

func (s *MapdSettings) LoadWithRetries(tries int) {
	for range tries {
		if s.Load() {
			break
		}
		time.Sleep(1 * time.Second)
	}
	s.Save()
}

func (s *MapdSettings) Save() {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		slog.Error("failed to marshal settings to json", "error", err)
		return
	}
	err = params.PutParam(params.MAPD_SETTINGS, data)
	if err != nil {
		slog.Error("failed to save MAPD_SETTINGS param", "error", err)
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

func (s *MapdSettings) GetDownloadProgress() (progress DownloadProgress, success bool) {
	select {
	case progress = <- s.downloadProgress:
		s.downloadActive = progress.Active
		return progress, true
	default:
	}
	return
}

func (s *MapdSettings) Handle(input custom.MapdIn) {
	switch input.Type() {
	case custom.MapdInputType_reloadSettings:
		s.Load()
	case custom.MapdInputType_saveSettings:
		go s.Save()
	case custom.MapdInputType_setVisionCurveMinTargetV:
		s.VisionCurveMinTargetV = input.Float()
	case custom.MapdInputType_setVisionCurveTargetLatA:
		s.VisionCurveTargetLatA = input.Float()
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
	case custom.MapdInputType_setVisionCurveUseEnableSpeed:
		s.VisionCurveUseEnableSpeed = input.Bool()
	case custom.MapdInputType_setCurveUseEnableSpeed:
		s.CurveUseEnableSpeed = input.Bool()
	case custom.MapdInputType_setSpeedLimitUseEnableSpeed:
		s.SpeedLimitUseEnableSpeed = input.Bool()
	case custom.MapdInputType_setHoldLastSeenSpeedLimit:
		s.HoldLastSeenSpeedLimit = input.Bool()
	case custom.MapdInputType_setCurveTargetJerk:
		s.CurveTargetJerk = input.Float()
	case custom.MapdInputType_setCurveTargetAccel:
		s.CurveTargetAccel = input.Float()
	case custom.MapdInputType_setCurveTargetOffset:
		s.CurveTargetOffset = input.Float()
	case custom.MapdInputType_setDefaultLaneWidth:
		s.DefaultLaneWidth = input.Float()
	case custom.MapdInputType_setCurveTargetLatA:
		s.CurveTargetLatA = input.Float()
	case custom.MapdInputType_setHoldSpeedLimitWhileChangingSetSpeed:
		s.HoldSpeedLimitWhileChangingSetSpeed = input.Bool()
	case custom.MapdInputType_setSpeedUpForNextSpeedLimit:
		s.SpeedUpForNextSpeedLimit = input.Bool()
	case custom.MapdInputType_setSlowDownForNextSpeedLimit:
		s.SlowDownForNextSpeedLimit = input.Bool()
	case custom.MapdInputType_loadPersistentSettings:
		s.Load()
	case custom.MapdInputType_loadDefaultSettings:
		s.Default()
	case custom.MapdInputType_loadRecommendedSettings:
		s.Recommended()
	case custom.MapdInputType_cancelDownload:
		select {
			case s.cancelDownload <- true:
			default:
		}
	case custom.MapdInputType_download:
		path, err := input.Str()
		if err != nil {
			slog.Warn("failed to read download path string", "error", err)
			return
		}
		if !s.downloadActive {
			go Download(path, s.downloadProgress, s.cancelDownload)
		}
	case custom.MapdInputType_setLogLevel:
		logLevel, err := input.Str()
		if err != nil {
			slog.Warn("failed to read log level string", "error", err)
			return
		}
		s.LogLevel = logLevel
		s.setLogLevel()
	}
}
