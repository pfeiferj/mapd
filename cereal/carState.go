package cereal

import (
	"github.com/pfeiferj/gomsgq"

	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal/car"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

type CarSubscriber struct {
	Sub gomsgq.MsgqSubscriber
}

func (s *CarSubscriber) Read() (model car.CarState, success bool) {
	data := s.Sub.Read()
	if len(data) == 0 {
		return model, false
	}
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return model, false
	}

	event, err := log.ReadRootEvent(msg)
	if err != nil {
		return model, false
	}

	model, err = event.CarState()
	if err != nil {
		return model, false
	}
	return model, true
}

func GetCarSub() (carSub CarSubscriber) {
	msgq := gomsgq.Msgq{}
	err := msgq.Init("carState", settings.DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	sub := gomsgq.MsgqSubscriber{}
	sub.Conflate = true
	sub.Init(msgq)

	carSub.Sub = sub
	return carSub
}
