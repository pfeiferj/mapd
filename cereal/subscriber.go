package cereal

import (
	"math"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/pfeiferj/gomsgq"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

type Reader[T any] func(log.Event) (T, error)

type SubscriberStatus struct {
	EventValid bool
	ReceivedAt time.Time
	Seen       bool
}

func (s SubscriberStatus) Healthy(now time.Time, maxAge time.Duration) bool {
	age := now.Sub(s.ReceivedAt)
	return s.Seen && s.EventValid && age >= 0 && age < maxAge
}

type Subscriber[T any] struct {
	Sub    gomsgq.MsgqSubscriber
	now    func() time.Time
	reader Reader[T]
	status SubscriberStatus
}

func (s *Subscriber[T]) Read() (obj T, success bool) {
	data := s.Sub.Read()
	if len(data) == 0 {
		return obj, false
	}
	receivedAt := s.nowTime()
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
	s.status = SubscriberStatus{
		EventValid: event.Valid(),
		ReceivedAt: receivedAt,
		Seen:       true,
	}
	return obj, true
}

func (s *Subscriber[T]) Status() SubscriberStatus {
	return s.status
}

func (s *Subscriber[T]) nowTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func NewSubscriber[T any](name string, reader Reader[T], conflate bool, shadow bool) (subscriber Subscriber[T]) {
	msgq := gomsgq.Msgq{}
	err := msgq.Init(name, settings.GetSegmentSize(name))
	if err != nil {
		panic(err)
	}
	sub := gomsgq.MsgqSubscriber{}
	sub.Conflate = conflate
	sub.Shadow = shadow
	sub.Init(msgq)

	subscriber.Sub = sub
	subscriber.now = time.Now
	subscriber.reader = reader
	return subscriber
}
