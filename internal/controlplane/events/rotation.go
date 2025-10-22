package events

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
)

// RotationSource abstracts subscription to GitLab rotation events.
type RotationSource interface {
	SubscribeRotations() *gitlab.RotationSubscription
}

// RotationHub fan-outs rotation events to control-plane consumers.
type RotationHub struct {
	ctx    context.Context
	cancel context.CancelFunc

	src RotationSource
	sub *gitlab.RotationSubscription

	mu      sync.Mutex
	waiters map[int64]*rotationWaiter
	last    map[string]gitlab.RotationEvent
	seq     atomic.Int64
	wg      sync.WaitGroup
}

type rotationWaiter struct {
	secret string
	since  int64
	ch     chan gitlab.RotationEvent
}

// NewRotationHub constructs a hub bound to the provided rotation source.
func NewRotationHub(ctx context.Context, src RotationSource) *RotationHub {
	if ctx == nil {
		ctx = context.Background()
	}
	hubCtx, cancel := context.WithCancel(ctx)
	hub := &RotationHub{
		ctx:     hubCtx,
		cancel:  cancel,
		src:     src,
		waiters: make(map[int64]*rotationWaiter),
		last:    make(map[string]gitlab.RotationEvent),
	}
	if src != nil {
		hub.sub = src.SubscribeRotations()
	}
	hub.wg.Add(1)
	go hub.run()
	return hub
}

// Wait blocks until a rotation event newer than the supplied revision is observed.
// If secret is empty, the next event for any secret is returned.
func (h *RotationHub) Wait(ctx context.Context, secret string, since int64) (gitlab.RotationEvent, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	secret = strings.TrimSpace(secret)

	h.mu.Lock()
	if evt, ok := h.matchLocked(secret, since); ok {
		h.mu.Unlock()
		return evt, true
	}
	id := h.seq.Add(1)
	waiter := &rotationWaiter{
		secret: secret,
		since:  since,
		ch:     make(chan gitlab.RotationEvent, 1),
	}
	h.waiters[id] = waiter
	h.mu.Unlock()

	select {
	case <-ctx.Done():
		h.mu.Lock()
		delete(h.waiters, id)
		h.mu.Unlock()
		return gitlab.RotationEvent{}, false
	case evt, ok := <-waiter.ch:
		if !ok {
			return gitlab.RotationEvent{}, false
		}
		return evt, true
	}
}

// Close releases hub resources and terminates the rotation subscription.
func (h *RotationHub) Close() {
	h.cancel()
	h.wg.Wait()
	if h.sub != nil {
		h.sub.Close()
	}
}

func (h *RotationHub) run() {
	defer h.wg.Done()
	if h.sub == nil {
		h.cleanupWaiters()
		return
	}
	defer h.cleanupWaiters()

	for {
		select {
		case <-h.ctx.Done():
			return
		case evt, ok := <-h.sub.C:
			if !ok {
				return
			}
			h.handleEvent(evt)
		}
	}
}

func (h *RotationHub) handleEvent(evt gitlab.RotationEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.last[evt.SecretName] = evt
	h.last[""] = evt

	for id, waiter := range h.waiters {
		if evt.Revision <= waiter.since {
			continue
		}
		if waiter.secret != "" && waiter.secret != evt.SecretName {
			continue
		}
		select {
		case waiter.ch <- evt:
		default:
		}
		delete(h.waiters, id)
	}
}

func (h *RotationHub) matchLocked(secret string, since int64) (gitlab.RotationEvent, bool) {
	if secret != "" {
		evt, ok := h.last[secret]
		if ok && evt.Revision > since {
			return evt, true
		}
		return gitlab.RotationEvent{}, false
	}
	evt, ok := h.last[""]
	if ok && evt.Revision > since {
		return evt, true
	}
	return gitlab.RotationEvent{}, false
}

func (h *RotationHub) cleanupWaiters() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, waiter := range h.waiters {
		close(waiter.ch)
		delete(h.waiters, id)
	}
}
