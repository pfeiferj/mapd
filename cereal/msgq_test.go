package cereal

import (
	"os"
	"path/filepath"
	"testing"

	"pfeifer.dev/mapd/settings"
)

func TestParseMaxReaders(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		want         int64
	}{
		{"legacy", "#define NUM_READERS 15\n", 15},
		{"new", "#define NUM_READERS 31\n", 31},
		{"whitespace", "// header\n  # define\tNUM_READERS\t63 // readers\n", 63},
		{"missing", "#define OTHER 31\n", defaultMaxReaders},
		{"zero", "#define NUM_READERS 0\n", defaultMaxReaders},
		{"negative", "#define NUM_READERS -1\n", defaultMaxReaders},
		{"overflow", "#define NUM_READERS 9223372036854775808\n", defaultMaxReaders},
		{"invalid", "#define NUM_READERS invalid\n", defaultMaxReaders},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseMaxReaders([]byte(tc.header)); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDetectMaxReaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "msgq.h")
	if got := detectMaxReaders(path); got != defaultMaxReaders {
		t.Fatalf("missing header: got %d, want %d", got, defaultMaxReaders)
	}
	if err := os.WriteFile(path, []byte("#define NUM_READERS 31\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := detectMaxReaders(path); got != 31 {
		t.Fatalf("got %d, want 31", got)
	}
}

func TestMaxReadersOverride(t *testing.T) {
	original := settings.Settings
	t.Cleanup(func() { settings.Settings = original })
	settings.Settings = settings.MapdSettings{MsgqQueueSize: 31}
	if got := maxReaders(); got != 31 {
		t.Fatalf("setting: got %d, want 31", got)
	}
	if err := settings.Settings.SetMsgqQueueSizeOverride(63); err != nil {
		t.Fatal(err)
	}
	if got := maxReaders(); got != 63 {
		t.Fatalf("override: got %d, want 63", got)
	}
	if err := settings.Settings.SetMsgqQueueSizeOverride(0); err != nil {
		t.Fatal(err)
	}
	if got, want := maxReaders(), detectMaxReaders(msgqHeaderPath); got != want {
		t.Fatalf("explicit auto: got %d, want %d", got, want)
	}
}
