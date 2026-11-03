package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMsgqQueueSizeSettings(t *testing.T) {
	var s MapdSettings
	if err := json.Unmarshal(defaultsJson, &s); err != nil {
		t.Fatal(err)
	}
	if size, err := s.EffectiveMsgqQueueSize(); err != nil || size != 0 {
		t.Fatalf("default: size=%d, err=%v", size, err)
	}
	s.MsgqQueueSize = 31
	if err := json.Unmarshal(recommendedJson, &s); err != nil {
		t.Fatal(err)
	}
	if size, err := s.EffectiveMsgqQueueSize(); err != nil || size != 0 {
		t.Fatalf("recommended: size=%d, err=%v", size, err)
	}
	if err := json.Unmarshal([]byte(`{"settings_version":2}`), &s); err != nil {
		t.Fatal(err)
	}
	if size, err := s.EffectiveMsgqQueueSize(); err != nil || size != 0 {
		t.Fatalf("older settings without key: size=%d, err=%v", size, err)
	}
	if err := json.Unmarshal([]byte(`{"msgq_queue_size":31}`), &s); err != nil {
		t.Fatal(err)
	}
	if size, err := s.EffectiveMsgqQueueSize(); err != nil || size != 31 {
		t.Fatalf("saved setting: size=%d, err=%v", size, err)
	}
	if err := s.SetMsgqQueueSizeOverride(0); err != nil {
		t.Fatal(err)
	}
	// A subsequent settings load must not overwrite the process-local override.
	if err := json.Unmarshal([]byte(`{"msgq_queue_size":63}`), &s); err != nil {
		t.Fatal(err)
	}
	if size, err := s.EffectiveMsgqQueueSize(); err != nil || size != 0 {
		t.Fatalf("explicit auto override: size=%d, err=%v", size, err)
	}
	data, err := json.Marshal(&s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"msgq_queue_size":63`) {
		t.Fatalf("override changed persisted setting: %s", data)
	}
}

func TestInvalidMsgqQueueSize(t *testing.T) {
	s := MapdSettings{MsgqQueueSize: -1}
	if _, err := s.EffectiveMsgqQueueSize(); err == nil {
		t.Fatal("negative setting accepted")
	}
	if err := s.SetMsgqQueueSizeOverride(-1); err == nil {
		t.Fatal("negative override accepted")
	}
	for _, value := range []string{"1.5", "9223372036854775808", `"31"`} {
		if err := json.Unmarshal([]byte(`{"msgq_queue_size":`+value+`}`), &s); err == nil {
			t.Fatalf("invalid value %s accepted", value)
		}
	}
}
