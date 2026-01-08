package cereal

import (
	"capnproto.org/go/capnp/v3"
	"github.com/pfeiferj/gomsgq"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

type MessageCreator[T any] func(log.Event) (T, error)

type Publisher[T any] struct {
	Pub     gomsgq.MsgqPublisher
	creator MessageCreator[T]
}

func (p *Publisher[T]) Send(msg *capnp.Message) error {
	b, err := msg.Marshal()
	if err != nil {
		return err
	}
	p.Pub.Send(b)
	return nil
}

func (p *Publisher[T]) NewMessage(valid bool) (msg *capnp.Message, obj T) {
	arena := capnp.SingleSegment(nil)

	msg, seg, err := capnp.NewMessage(arena)
	if err != nil {
		panic(err)
	}

	event, err := log.NewRootEvent(seg)
	if err != nil {
		panic(err)
	}

	event.SetLogMonoTime(GetTime())
	event.SetValid(valid)

	obj, err = p.creator(event)
	if err != nil {
		panic(err)
	}

	return msg, obj
}

func NewPublisher[T any](name string, creator MessageCreator[T]) (publisher Publisher[T]) {
	msgq := gomsgq.Msgq{}
	err := msgq.Init(name, settings.GetSegmentSize(name))
	if err != nil {
		panic(err)
	}
	pub := gomsgq.MsgqPublisher{}
	pub.Init(msgq)

	publisher.Pub = pub
	publisher.creator = creator
	return publisher
}
