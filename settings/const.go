package settings

import (
	"time"
)

const (
	DEFAULT_SEGMENT_SIZE = 10 * 1024 * 1024
	LOOP_DELAY           = 50 * time.Millisecond
	VTSC_TARGET_LAT_A    = 1.9    // m/s^2
	VTSC_MIN_TARGET_V    = 5      // m/s
	LIMIT_OFFSET         = 2.2352 // TODO do not hardcode
)
