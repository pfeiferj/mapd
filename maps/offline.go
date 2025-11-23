package maps

import (
	"log/slog"

	"pfeifer.dev/mapd/cereal/offline"
	m "pfeifer.dev/mapd/math"
	u "pfeifer.dev/mapd/utils"

	"capnproto.org/go/capnp/v3"
)

func ReadOffline(data []uint8) Offline {
	msg, err := capnp.UnmarshalPacked(data)
	if err != nil {
		slog.Warn("could not unmarshal offline data", "error", err)
	}
	if err == nil {
		offlineMaps, err := offline.ReadRootOffline(msg)
		if err != nil {
			slog.Warn("could not read offline message", "error", err)
		}
		return Offline{offline: offlineMaps}
	}
	return Offline{}
}

type Offline struct {
	offline offline.Offline
	box        u.Curry[m.Box]
	overlapBox u.Curry[m.Box]
	ways	     u.Curry[[]Way]
	overlap    u.Curry[float64]
}

func (o *Offline) _box() m.Box {
	return m.Box{
		MinPos: m.NewPosition(float64(o.offline.MinLat()), float64(o.offline.MinLon())),
		MaxPos: m.NewPosition(float64(o.offline.MaxLat()), float64(o.offline.MaxLon())),
	}
}

func (o *Offline) Box() m.Box {
	return o.box.Value(o._box)
}

func (o *Offline) _overlapBox() m.Box {
	box := o.Box()
	return box.Overlap(o.Overlap())
}

func (o *Offline) OverlapBox() m.Box {
	return o.overlapBox.Value(o._overlapBox)
}

func (o *Offline) _overlap() float64 {
	return o.offline.Overlap()
}

func (o *Offline) Overlap() float64 {
	return o.overlap.Value(o._overlap)
}

func (o *Offline) _ways() []Way {
	ways, err := o.offline.Ways()
	if err != nil {
		slog.Warn("Could not read ways from offline maps", "error", err)
	}
	res := make([]Way, ways.Len())
	for i := range ways.Len() {
		res[i].Way = ways.At(i)
	}
	return res
}

func (o *Offline) Ways() []Way {
	return o.ways.Value(o._ways)
}
