package cereal

import (
	"os"
	"regexp"
	"strconv"
)

const (
	defaultMaxReaders = 15
	msgqHeaderPath    = "/data/openpilot/msgq_repo/msgq/msgq.h"
)

var numReadersPattern = regexp.MustCompile(`(?m)^\s*#\s*define\s+NUM_READERS\s+([0-9]+)\b`)

// maxReaders returns openpilot's configured msgq reader capacity. Mapd must
// create a queue with the same capacity as the other processes sharing it.
func maxReaders() int64 {
	contents, err := os.ReadFile(msgqHeaderPath)
	if err != nil {
		return defaultMaxReaders
	}

	matches := numReadersPattern.FindSubmatch(contents)
	if len(matches) != 2 {
		return defaultMaxReaders
	}

	readers, err := strconv.ParseInt(string(matches[1]), 10, 64)
	if err != nil || readers <= 0 {
		return defaultMaxReaders
	}
	return readers
}
