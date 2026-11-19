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
		{"AT:shared_zone20", "20"},
		{"DE:shared_zone:30 mph", "30 mph"},
		{"AT:city_limit:40", "40"},
		{"DE:city_limit30 mph", "30 mph"},
		{"AT:city_limit", ""},
		{"A:shared_zone20", ""},
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

func TestParseWayMaxSpeed(t *testing.T) {
	for _, tc := range []struct {
		name, maxspeed, source, speedType, want string
	}{
		{"prefer explicit maxspeed", "30 mph", "DE:urban", "DE:rural", "30 mph"},
		{"prefer source over type", "unknown", "DE:zone30 mph", "DE:zone:20", "30 mph"},
		{"missing maxspeed", "", "DE:zone30", "", "30"},
		{"unparseable source", "unknown", "sign", "DE:zone:20", "20"},
		{"only type", "", "", "DE:zone:20", "20"},
		{"urban in maxspeed", "DE:urban", "30", "DE:rural", "50"},
		{"rural in source", "", "DE:rural", "DE:urban", "100"},
		{"regional type", "", "sign", "BE-BRU:urban", "30"},
		{"different regional default", "BE-WAL:rural", "", "", "90"},
		{"provincial default", "CA-BC:rural", "", "", "80"},
		{"road-class variant", "AR:urban:primary", "", "", "60"},
		{"ambiguous value falls through", "ES:urban", "CA-QC:rural", "FR:urban", "50"},
		{"unknown country", "XX:rural", "", "", ""},
		{"motorway in maxspeed", "AT:motorway", "50", "", "130"},
		{"living street in source", "", "BE:living_street", "BE:trunk", "20"},
		{"Bicycle road in type", "", "sign", "DE:bicycle_road", "30"},
		{"regional motorway", "CA-QC:motorway", "", "", "100"},
		{"provincial national limit", "CA-SK:nsl", "", "", "80"},
		{"restricted mph", "GB:nsl_restricted", "", "", "30 mph"},
		{"single carriageway mph", "GB:nsl_single", "", "", "60 mph"},
		{"legacy dual carriageway mph", "", "", "UK:nsl_dual", "70 mph"},
		{"shared zone with number", "AT:shared_zone20", "", "", "20"},
		{"city limit with number", "", "AT:city_limit40", "", "40"},
		{"city limit without number", "AT:city_limit", "AT:urban", "", "50"},
		{"variable motorway limits fall through", "FR:motorway", "TR:motorway", "130", "130"},
		{"no usable speed", "DE:living_street", "sign", "DE:motorway", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tags := map[string]string{
				"maxspeed":        tc.maxspeed,
				"source:maxspeed": tc.source,
				"maxspeed:type":   tc.speedType,
			}
			if got, want := parseWayMaxSpeed(tags), ParseMaxSpeed(tc.want); got != want {
				t.Errorf("parseWayMaxSpeed(%v) = %v m/s; want %v m/s", tags, got, want)
			}
		})
	}
}
