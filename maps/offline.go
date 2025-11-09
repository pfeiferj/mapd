package maps

import (
	"pfeifer.dev/mapd/cereal/offline"
	"pfeifer.dev/mapd/utils"

	"github.com/pkg/errors"
	"capnproto.org/go/capnp/v3"
)

func ReadOffline(data []uint8) offline.Offline {
	msg, err := capnp.UnmarshalPacked(data)
	utils.Logde(errors.Wrap(err, "could not unmarshal offline data"))
	if err == nil {
		offlineMaps, err := offline.ReadRootOffline(msg)
		utils.Logde(errors.Wrap(err, "could not read offline message"))
		return offlineMaps
	}
	return offline.Offline{}
}

