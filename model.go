package main

import (
	"github.com/pfeiferj/gomsgq"

	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal/log"
)

type ModelSubscriber struct {
	Sub  gomsgq.MsgqSubscriber
}

func (s *ModelSubscriber) Read() (model log.ModelDataV2, success bool) {
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

	model, err = event.ModelV2()
	if err != nil {
		return model, false
	}
	return model, true
}

func getModelSub() (modelSub ModelSubscriber) {
	msgq := gomsgq.Msgq{}
	err := msgq.Init("modelV2", DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	sub := gomsgq.MsgqSubscriber{}
	sub.Init(msgq)
	sub.Conflate = true

	modelSub.Sub = sub
	return modelSub
}
