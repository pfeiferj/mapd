package maps

import "testing"

func TestParseMaxSpeedZone(t *testing.T) {
	for _, tc := range []struct {
		input      string
		standalone string
	}{
		{"DE:zone:30", "30"},
		{"DE:zone30", "30"},
		{"DE:zone30 mph", "30 mph"},
		{"DE:zoneurban", ""},
		{"DE:zone::30", ""},
		{"AT:zone:50", "50"},
		{"DE:zone:urban", ""},
		{"DE:zone:30 mph", "30 mph"},
		{"DE:zone:30 km/h", "30 km/h"},
		{"DE:zone:30 kph", "30 kph"},
		{"DE:zone:30 kmh", "30 kmh"},
		{"DE:zone:30 knots", "30 knots"},
		{"DE:zone:30 unknown", ""},
		{"DE:zone:-30", ""},
		{"DE:urban", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var want float64
			if tc.standalone != "" {
				want = ParseMaxSpeed(tc.standalone)
			}
			if got := ParseMaxSpeed(tc.input); got != want {
				t.Errorf("ParseMaxSpeed(%q) = %v m/s; want %v m/s", tc.input, got, want)
			}
		})
	}
}
