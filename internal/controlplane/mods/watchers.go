package mods

import "context"

// JobCompletionWatcher streams job completion events to the orchestrator.
type JobCompletionWatcher interface {
	Watch(ctx context.Context) (<-chan JobCompletion, error)
}

// startWatchers activates background watchers when configured.
func (s *Service) startWatchers() {
	if s.watcher == nil {
		return
	}
	s.wg.Add(1)
	go s.watchJobCompletions()
}

// watchJobCompletions forwards completion events to the service dispatcher.
func (s *Service) watchJobCompletions() {
	defer s.wg.Done()
	ch, err := s.watcher.Watch(s.ctx)
	if err != nil {
		return
	}
	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			_ = s.ProcessJobCompletion(s.ctx, event)
		}
	}
}
