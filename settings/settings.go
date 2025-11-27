package settings

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"

	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/params"
)

var Settings = MapdSettings{
	downloadProgress: make(chan DownloadProgress, 1),
	cancelDownload: make(chan bool, 1),
}

type SpeedLimitPriority string

const (
	PRIORITY_MAP = "map"
	PRIORITY_EXTERNAL = "external"
	PRIORITY_HIGHEST = "highest"
	PRIORITY_LOWEST = "lowest"
)

type MapdSettings struct {
	downloadProgress                    chan DownloadProgress
	cancelDownload                      chan bool
	downloadActive                      bool
	externalSpeedLimit									float32
	speedLimitAccepted									bool
	VisionCurveSpeedControlEnabled      bool    `json:"vision_curve_speed_control_enabled"`
	CurveSpeedControlEnabled            bool    `json:"curve_speed_control_enabled"`
	SpeedLimitControlEnabled            bool    `json:"speed_limit_control_enabled"`
	ExternalSpeedLimitControlEnabled    bool    `json:"external_speed_limit_control_enabled"`
	SpeedLimitPriority                  string  `json:"speed_limit_priority"`
	VisionCurveUseEnableSpeed           bool    `json:"vision_curve_use_enable_speed"`
	SpeedLimitUseEnableSpeed            bool    `json:"speed_limit_use_enable_speed"`
	SpeedLimitChangeRequiresAccept      bool    `json:"speed_limit_change_requires_accept"`
	CurveUseEnableSpeed                 bool    `json:"curve_use_enable_speed"`
	LogLevel                            string  `json:"log_level"`
	LogJson                             bool    `json:"log_json"`
	LogSource                           bool    `json:"log_source"`
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
	if _, err := os.Stat("/data/openpilot/mapd_defaults.json"); err == nil {
		defaults, err := os.ReadFile("/data/openpilot/mapd_defaults.json")
		if err != nil {
			slog.Warn("failed to read custom default settings", "error", err)
		}
		err = json.Unmarshal(defaults, s)
		if err != nil {
			slog.Warn("failed to load custom default settings", "error", err)
			return
		}
	} else {
		err := json.Unmarshal(defaultsJson, s)
		if err != nil {
			slog.Warn("failed to load default settings", "error", err)
			return
		}
	}
}

func (s *MapdSettings) Recommended() {
	if _, err := os.Stat("/data/openpilot/mapd_recommended.json"); err == nil {
		recommended, err := os.ReadFile("/data/openpilot/mapd_recommended.json")
		if err != nil {
			slog.Warn("failed to read custom recommended settings", "error", err)
		}
		err = json.Unmarshal(recommended, s)
		if err != nil {
			slog.Warn("failed to load custom recommended settings", "error", err)
			return
		}
	} else {
		err := json.Unmarshal(recommendedJson, s)
		if err != nil {
			slog.Warn("failed to load recommended settings", "error", err)
		}
	}
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

	s.setupLogger()

	return true
}

func (s *MapdSettings) Unmarshal(b []byte) (success bool) {
	s.Default() // set defaults so settings not already in param are defaulted
	err := json.Unmarshal(b, s)
	if err != nil {
		slog.Warn("failed to unmarshal settings data", "error", err)
		return false
	}

	s.setupLogger()

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

func (s *MapdSettings) setupLogger() {
	handlerOptions := slog.HandlerOptions{
		AddSource: s.LogSource,
	}
	if s.LogJson {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &handlerOptions)))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &handlerOptions)))
	}
	s.setLogLevel()
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

func (s *MapdSettings) ExternalSpeedLimit() float32 {
	return s.externalSpeedLimit
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
	case custom.MapdInputType_setExternalSpeedLimitControl:
		s.ExternalSpeedLimitControlEnabled = input.Bool()
	case custom.MapdInputType_setSpeedLimitPriority:
		priority, err := input.Str()
		if err != nil {
			slog.Warn("failed to read speed limit priority string", "error", err)
			return
		}
		s.SpeedLimitPriority = priority
	case custom.MapdInputType_setExternalSpeedLimit:
		s.externalSpeedLimit = input.Float()
	case custom.MapdInputType_setHoldSpeedLimitWhileChangingSetSpeed:
		s.HoldSpeedLimitWhileChangingSetSpeed = input.Bool()
	case custom.MapdInputType_setSpeedUpForNextSpeedLimit:
		s.SpeedUpForNextSpeedLimit = input.Bool()
	case custom.MapdInputType_setSlowDownForNextSpeedLimit:
		s.SlowDownForNextSpeedLimit = input.Bool()
	case custom.MapdInputType_acceptSpeedLimit:
		s.AcceptSpeedLimit()
	case custom.MapdInputType_setLogSource:
		s.LogSource = input.Bool()
		s.setupLogger()
	case custom.MapdInputType_setLogJson:
		s.LogJson = input.Bool()
		s.setupLogger()
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

func (s *MapdSettings) PrioritySpeedLimit(mapLimit float32) float32 {
	if s.SpeedLimitControlEnabled && !s.ExternalSpeedLimitControlEnabled {
		return mapLimit
	}
	if s.ExternalSpeedLimitControlEnabled && !s.SpeedLimitControlEnabled {
		return s.externalSpeedLimit
	}
	if !s.SpeedLimitControlEnabled && !s.ExternalSpeedLimitControlEnabled {
		return 0
	}
	switch s.SpeedLimitPriority {
	case PRIORITY_MAP:
		if mapLimit == 0 {
			return s.externalSpeedLimit
		}
		return mapLimit
	case PRIORITY_EXTERNAL:
		if s.externalSpeedLimit == 0 {
			return mapLimit
		}
		return s.externalSpeedLimit
	case PRIORITY_HIGHEST:
		return max(mapLimit, s.externalSpeedLimit)
	case PRIORITY_LOWEST:
		if mapLimit == 0 {
			return s.externalSpeedLimit
		}
		if s.externalSpeedLimit == 0 {
			return mapLimit
		}
		return min(mapLimit, s.externalSpeedLimit)
	default:
		if mapLimit == 0 {
			return s.externalSpeedLimit
		}
		return mapLimit
	}
}

func (s *MapdSettings) ResetSpeedLimitAccepted() {
	s.speedLimitAccepted = false
}

func (s *MapdSettings) SpeedLimitAccepted() bool {
	if !s.SpeedLimitChangeRequiresAccept {
		return true
	}
	return s.speedLimitAccepted
}

func (s *MapdSettings) AcceptSpeedLimit() {
	s.speedLimitAccepted = true
}
