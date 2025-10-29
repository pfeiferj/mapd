package cereal

import (
	"github.com/pfeiferj/gomsgq"
	"pfeifer.dev/mapd/settings"
	"log/slog"
)

func GetMapdPub() gomsgq.MsgqPublisher {
	slog.Info("is it global?", "v", settings.Settings.VtscMinTargetV) 
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
