package cereal

import (
	"github.com/pfeiferj/gomsgq"

	"pfeifer.dev/mapd/settings"
	"capnproto.org/go/capnp/v3"
	"pfeifer.dev/mapd/cereal/custom"
	"pfeifer.dev/mapd/cereal/log"
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

func GetMapdCliPub() gomsgq.MsgqPublisher {
	msgq := gomsgq.Msgq{}
	err := msgq.Init("mapdCli", settings.DEFAULT_SEGMENT_SIZE)
	if err != nil {
		panic(err)
	}
	pub := gomsgq.MsgqPublisher{}
	pub.Init(msgq)

	return pub
}

func NewOutput() (*capnp.Message, log.Event, custom.MapdOut) {
	arena := capnp.SingleSegment(nil)

	msg, seg, err := capnp.NewMessage(arena)
	if err != nil {
		panic(err)
	}

	event, err := log.NewRootEvent(seg)
	if err != nil {
		panic(err)
	}
	mapdOut, err := event.NewMapdOut()
	if err != nil {
		panic(err)
	}
	event.SetLogMonoTime(GetTime())

	return msg, event, mapdOut
}
