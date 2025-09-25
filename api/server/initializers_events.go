package server

import (
	"context"
	"log"
	"strings"
	"time"

	modsapi "github.com/iw2rmb/ploy/api/mods"
	eventsfabric "github.com/iw2rmb/ploy/internal/events/fabric"
	"github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/nats-io/nats.go"
)

func initializeEventFabric(cfg *ControllerConfig, deps *ServiceDependencies) (*eventsfabric.Fabric, error) {
	eventsCfg := cfg.JetStreamEvents
	if !eventsCfg.Enabled || strings.TrimSpace(eventsCfg.URL) == "" {
		log.Printf("Event fabric disabled (URL=%q)", eventsCfg.URL)
		SetBuildStatusPublisher(nil)
		orchestration.SetReadyNotifier(nil)
		if deps != nil && deps.ModsHandler != nil {
			deps.ModsHandler.SetEventPublisher(nil)
		}
		return nil, nil
	}

	opts := []nats.Option{nats.Name("ploy-event-fabric")}
	if eventsCfg.CredentialsPath != "" {
		opts = append(opts, nats.UserCredentials(eventsCfg.CredentialsPath))
	}
	if eventsCfg.User != "" {
		opts = append(opts, nats.UserInfo(eventsCfg.User, eventsCfg.Password))
	}

	conn, err := nats.Connect(eventsCfg.URL, opts...)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fabricCfg := eventsfabric.Config{
		MaxAge:             eventsCfg.MaxAge,
		BuildStream:        eventsCfg.BuildStream,
		BuildSubjectPrefix: eventsCfg.BuildSubjectPrefix,
		BuildReplicas:      eventsCfg.BuildReplicas,
		AllocStream:        eventsCfg.AllocStream,
		AllocSubjectPrefix: eventsCfg.AllocSubjectPrefix,
		AllocReplicas:      eventsCfg.AllocReplicas,
		ModsStream:         eventsCfg.ModsStream,
		ModsSubjectPrefix:  eventsCfg.ModsSubjectPrefix,
		ModsReplicas:       eventsCfg.ModsReplicas,
	}

	fabric, err := eventsfabric.New(ctx, conn, fabricCfg)
	if err != nil {
		conn.Close()
		return nil, err
	}

	SetBuildStatusPublisher(func(ctx context.Context, st buildStatus) {
		if ctx == nil {
			ctx = context.Background()
		}
		ev := eventsfabric.BuildStatusEvent{
			ID:        st.ID,
			App:       st.App,
			Status:    st.Status,
			Code:      st.Code,
			Message:   st.Message,
			StartedAt: st.StartedAt,
			EndedAt:   st.EndedAt,
		}
		if err := fabric.PublishBuildStatus(ctx, ev); err != nil {
			log.Printf("failed to publish build status event: %v", err)
		}
	})

	orchestration.SetReadyNotifier(func(ctx context.Context, jobID string, alloc *orchestration.AllocationStatus, healthy int) {
		if ctx == nil {
			ctx = context.Background()
		}
		if alloc == nil {
			return
		}
		ev := eventsfabric.AllocationReadyEvent{
			JobID:         jobID,
			AllocID:       alloc.ID,
			ClientStatus:  alloc.ClientStatus,
			DesiredStatus: alloc.DesiredStatus,
			HealthyCount:  healthy,
		}
		if alloc.TaskStates != nil {
			summary := make(map[string]string, len(alloc.TaskStates))
			for name, st := range alloc.TaskStates {
				if st == nil || st.State == "" {
					continue
				}
				summary[name] = st.State
			}
			if len(summary) > 0 {
				ev.TaskSummaries = summary
			}
		}
		if err := fabric.PublishAllocationReady(ctx, ev); err != nil {
			log.Printf("failed to publish allocation readiness: %v", err)
		}
	})

	if deps != nil && deps.ModsHandler != nil {
		deps.ModsHandler.SetEventPublisher(func(ctx context.Context, ev modsapi.ModEvent) {
			if ctx == nil {
				ctx = context.Background()
			}
			payload := eventsfabric.ModsEvent{
				ModID:   ev.ModID,
				Phase:   ev.Phase,
				Step:    ev.Step,
				Level:   ev.Level,
				Message: ev.Message,
				JobName: ev.JobName,
				AllocID: ev.AllocID,
			}
			if !ev.Time.IsZero() {
				payload.Time = ev.Time
			}
			if err := fabric.PublishModsEvent(ctx, payload); err != nil {
				log.Printf("failed to publish mods telemetry: %v", err)
			}
		})
	}

	return fabric, nil
}
