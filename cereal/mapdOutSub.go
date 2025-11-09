package cereal

import (
	"github.com/pfeiferj/gomsgq"

	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

type MapdOutSubscriber struct {
	Sub gomsgq.MsgqSubscriber
}

func (s *MapdOutSubscriber) Read() (input custom.MapdOut, success bool) {
	data := s.Sub.Read()
	if len(data) == 0 {
		return input, false
	}
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return input, false
	}

	event, err := log.ReadRootEvent(msg)
	if err != nil {
		return input, false
	}

	input, err = event.MapdOut()
	if err != nil {
		return input, false
	}
	return input, true
}

func GetMapdOutSub() (mapdSub MapdOutSubscriber) {
	msgq := gomsgq.Msgq{}
	err := msgq.Init("mapdOut", settings.DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	sub := gomsgq.MsgqSubscriber{}
	sub.Conflate = true
	sub.Init(msgq)

	mapdSub.Sub = sub
	return mapdSub
}
