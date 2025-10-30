package settings

import (
	"time"
)

const (
	DEFAULT_SEGMENT_SIZE = 10 * 1024 * 1024
	LOOP_DELAY           = 50 * time.Millisecond
	MS_TO_KPH            = 3.6
	KPH_TO_MS            = 1/3.6
	ENABLE_SPEED_RANGE   = 0.2 // m/s
)
