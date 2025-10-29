package cereal

import (
	"github.com/pfeiferj/gomsgq"

	"pfeifer.dev/mapd/settings"
)

func GetMapdPub() gomsgq.MsgqPublisher {
	msgq := gomsgq.Msgq{}
	err := msgq.Init("mapdOut", settings.DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	pub := gomsgq.MsgqPublisher{}
	pub.Init(msgq)

	return pub
}

func GetMapdInputPub() gomsgq.MsgqPublisher {
	msgq := gomsgq.Msgq{}
	err := msgq.Init("mapdIn", settings.DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	pub := gomsgq.MsgqPublisher{}
	pub.Init(msgq)

	return pub
}
