package cli

import (
	"context"
	"os"
	"testing"

	"github.com/urfave/cli/v3"
	"pfeifer.dev/mapd/settings"
)

func TestMsgqQueueSizeFlag(t *testing.T) {
	original := settings.Settings
	t.Cleanup(func() { settings.Settings = original })
	for _, tc := range []struct {
		name, env string
		args      []string
		want      int64
		wantErr   bool
	}{
		{name: "setting", want: 15},
		{name: "environment", env: "31", want: 31},
		{name: "flag", args: []string{"--msgq-queue-size", "63"}, want: 63},
		{name: "flag over environment", env: "31", args: []string{"--msgq-queue-size", "63"}, want: 63},
		{name: "environment auto", env: "0", want: 0},
		{name: "flag auto", env: "31", args: []string{"--msgq-queue-size=0"}, want: 0},
		{name: "interactive", env: "31", args: []string{"interactive"}, want: 31},
		{name: "interactive flag", args: []string{"interactive", "--msgq-queue-size=63"}, want: 63},
		{name: "negative flag", args: []string{"--msgq-queue-size=-1"}, wantErr: true},
		{name: "negative environment", env: "-1", wantErr: true},
		{name: "invalid environment", env: "bad", wantErr: true},
		{name: "fractional flag", args: []string{"--msgq-queue-size=1.5"}, wantErr: true},
		{name: "overflow", args: []string{"--msgq-queue-size=9223372036854775808"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MAPD_MSGQ_QUEUE_SIZE", tc.env)
			if tc.env == "" {
				if err := os.Unsetenv("MAPD_MSGQ_QUEUE_SIZE"); err != nil {
					t.Fatal(err)
				}
			}
			settings.Settings = settings.MapdSettings{MsgqQueueSize: 15}
			action := func(context.Context, *cli.Command) error { return nil }
			cmd := &cli.Command{
				Flags:    []cli.Flag{msgqQueueSizeFlag()},
				Before:   configureMsgqQueueSize,
				Action:   action,
				Commands: []*cli.Command{{Name: "interactive", Action: action}},
			}
			err := cmd.Run(context.Background(), append([]string{"mapd"}, tc.args...))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%t", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			got, err := settings.Settings.EffectiveMsgqQueueSize()
			if err != nil || got != tc.want {
				t.Fatalf("got %d, err=%v; want %d", got, err, tc.want)
			}
		})
	}
}
