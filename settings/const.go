package settings

import (
	"time"
	"math"
)

const (
	DEFAULT_SEGMENT_SIZE = 10 * 1024 * 1024
	LOOP_DELAY           = 50 * time.Millisecond
	MS_TO_KPH            = 3.6
	MS_TO_MPH            = 2.237
	KPH_TO_MS            = 1/MS_TO_KPH
	MPH_TO_MS            = 1/MS_TO_MPH
	ENABLE_SPEED_RANGE   = 0.2 // m/s
	HOLD_LAST_SEEN_SPEED = true //TODO move to settings
	R                    = 6373000.0 // approximate radius of earth in meters
	TO_RADIANS           = math.Pi / 180
	TO_DEGREES           = 180 / math.Pi
	TARGET_JERK          = -0.6
	TARGET_ACCEL         = -1.2
	TARGET_OFFSET        = 1.0
	LANE_WIDTH           = 3.7                 // meters
	QUERY_RADIUS         = float64(3000)       // meters
	PADDING              = 10 / R * TO_DEGREES // 10 meters in degrees
	TARGET_LAT_ACCEL     = 2.0 // m/s^2 //TODO move to settings
)
