package events

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
)

func TestRotationHubDeliversEvents(t *testing.T) {
	t.Helper()

	source := newFakeRotationSource()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewRotationHub(ctx, source)
	defer hub.Close()

	go func() {
		source.emit(gitlab.RotationEvent{SecretName: "deploy", Revision: 10, UpdatedAt: time.Now()})
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()

	evt, ok := hub.Wait(waitCtx, "", 0)
	if !ok {
		t.Fatalf("expected rotation event")
	}
	if evt.SecretName != "deploy" || evt.Revision != 10 {
		t.Fatalf("unexpected event: %+v", evt)
	}
}

func TestRotationHubSinceFilter(t *testing.T) {
	source := newFakeRotationSource()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewRotationHub(ctx, source)
	defer hub.Close()

	go func() {
		source.emit(gitlab.RotationEvent{SecretName: "deploy", Revision: 5, UpdatedAt: time.Now()})
		time.Sleep(50 * time.Millisecond)
		source.emit(gitlab.RotationEvent{SecretName: "deploy", Revision: 12, UpdatedAt: time.Now()})
	}()

	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()

	evt, ok := hub.Wait(waitCtx, "", 10)
	if !ok {
		t.Fatalf("expected rotation event after since filter")
	}
	if evt.Revision != 12 {
		t.Fatalf("expected revision 12, got %d", evt.Revision)
	}
}

func TestRotationHubSecretFilter(t *testing.T) {
	source := newFakeRotationSource()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewRotationHub(ctx, source)
	defer hub.Close()

	go func() {
		source.emit(gitlab.RotationEvent{SecretName: "runner", Revision: 3})
		time.Sleep(20 * time.Millisecond)
		source.emit(gitlab.RotationEvent{SecretName: "deploy", Revision: 4})
	}()

	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()

	evt, ok := hub.Wait(waitCtx, "deploy", 0)
	if !ok {
		t.Fatalf("expected deploy event")
	}
	if evt.SecretName != "deploy" {
		t.Fatalf("expected secret deploy, got %s", evt.SecretName)
	}
}

type fakeRotationSource struct {
	events chan gitlab.RotationEvent
}

func newFakeRotationSource() *fakeRotationSource {
	return &fakeRotationSource{
		events: make(chan gitlab.RotationEvent, 16),
	}
}

func (f *fakeRotationSource) SubscribeRotations() *gitlab.RotationSubscription {
	ch := make(chan gitlab.RotationEvent, 16)
	go func() {
		defer close(ch)
		for evt := range f.events {
			select {
			case ch <- evt:
			default:
			}
		}
	}()
	return &gitlab.RotationSubscription{C: ch}
}

func (f *fakeRotationSource) emit(evt gitlab.RotationEvent) {
	f.events <- evt
}

func (f *fakeRotationSource) Close() {
	close(f.events)
}
