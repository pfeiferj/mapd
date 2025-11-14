package maps

import (
	"log/slog"

	"pfeifer.dev/mapd/cereal/offline"

	"capnproto.org/go/capnp/v3"
)

func ReadOffline(data []uint8) offline.Offline {
	msg, err := capnp.UnmarshalPacked(data)
	if err != nil {
		slog.Warn("could not unmarshal offline data", "error", err)
	}
	if err == nil {
		offlineMaps, err := offline.ReadRootOffline(msg)
		if err != nil {
			slog.Warn("could not read offline message", "error", err)
		}
		return offlineMaps
	}
	return offline.Offline{}
}

