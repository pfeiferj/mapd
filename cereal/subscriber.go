package cereal

import (
	"math"

	"capnproto.org/go/capnp/v3"
	"github.com/pfeiferj/gomsgq"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

type Reader[T any] func(log.Event) (T, error)

type Subscriber[T any] struct {
	Sub    gomsgq.MsgqSubscriber
	reader Reader[T]
}

func (s *Subscriber[T]) Read() (obj T, success bool) {
	data := s.Sub.Read()
	if len(data) == 0 {
		return obj, false
	}
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return obj, false
	}

	// allow us to read as much as we want
	msg.ResetReadLimit(math.MaxUint64)

	event, err := log.ReadRootEvent(msg)
	if err != nil {
		return obj, false
	}

	obj, err = s.reader(event)
	if err != nil {
		return obj, false
	}
	return obj, true
}

func NewSubscriber[T any](name string, reader Reader[T], conflate bool) (subscriber Subscriber[T]) {
	msgq := gomsgq.Msgq{}
	err := msgq.Init(name, settings.GetSegmentSize(name))
	if err != nil {
		panic(err)
	}
	sub := gomsgq.MsgqSubscriber{}
	sub.Conflate = conflate
	sub.Init(msgq)

	subscriber.Sub = sub
	subscriber.reader = reader
	return subscriber
}
