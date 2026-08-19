package cereal

import (
	"log/slog"
	"time"

	"capnproto.org/go/capnp/v3"
	"github.com/pfeiferj/gomsgq"
	"pfeifer.dev/mapd/cereal/log"
	"pfeifer.dev/mapd/settings"
)

type MessageCreator[T any] func(log.Event) (T, error)

type Publisher[T any] struct {
	Pub     gomsgq.MsgqPublisher
	creator MessageCreator[T]

	msgChan chan *capnp.Message
	stop    chan struct{}
	done    chan struct{}
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

// StartAutoPublish starts a background loop that publishes at the given
// rate, decoupling the on-wire send cadence from however often the caller
// invokes Publish. Call it once before using Publish.
func (p *Publisher[T]) StartAutoPublish(rate time.Duration) {
	p.msgChan = make(chan *capnp.Message, 1)
	p.stop = make(chan struct{})
	p.done = make(chan struct{})
	go p.autoPublishLoop(rate)
}

// Publish hands msg to the auto-publish loop, replacing any message queued
// since the last tick. It never blocks. Only the newest message queued
// between ticks is ever sent. Requires StartAutoPublish to have been called
// first.
func (p *Publisher[T]) Publish(msg *capnp.Message) {
	for {
		select {
		case p.msgChan <- msg:
			return
		default:
			select {
			case <-p.msgChan:
			default:
			}
		}
	}
}

// Stop halts the auto-publish loop started by StartAutoPublish, if any.
func (p *Publisher[T]) Stop() {
	if p.stop == nil {
		return
	}
	close(p.stop)
	<-p.done
}

func (p *Publisher[T]) autoPublishLoop(rate time.Duration) {
	defer close(p.done)
	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	var lastMsg *capnp.Message
	for {
		select {
		case <-p.stop:
			return
		case msg := <-p.msgChan:
			lastMsg = msg
		case <-ticker.C:
			if lastMsg == nil {
				continue
			}
			// No new message arrived since the last tick: resend the
			// previous one with a refreshed timestamp rather than drop it.
			event, err := log.ReadRootEvent(lastMsg)
			if err != nil {
				slog.Error("failed to read event for auto-publish", "error", err)
				continue
			}
			event.SetLogMonoTime(GetTime())

			if err := p.Send(lastMsg); err != nil {
				slog.Error("failed to auto-publish message", "error", err)
			}
		}
	}
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
