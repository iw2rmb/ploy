package nodeagent

import (
	"context"
	"fmt"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func (r *runController) recordImageUse(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.imageLastUsed == nil {
		r.imageLastUsed = make(map[string]time.Time)
	}
	r.imageLastUsed[id] = time.Now()
}

func (r *runController) invalidateImageUse() {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A failed inspection can hide use of an already tracked image. Restarting
	// every grace period prevents an old observation from authorizing deletion.
	r.imageLastUsed = nil
}

// pruneCleanupImages requires exclusive maintenance protection. Docker image
// creation and tag timestamps do not describe use by short-lived test containers.
func (r *runController) pruneCleanupImages(ctx context.Context, docker cleanupDockerClient, containers []container.InspectResponse, now time.Time, age time.Duration) error {
	listed, err := docker.ImageList(ctx, client.ImageListOptions{All: true})
	if err != nil {
		return fmt.Errorf("list cleanup images: %w", err)
	}
	referenced := make(map[string]bool)
	for _, c := range containers {
		inspected, err := docker.ImageInspect(ctx, c.Image)
		if err != nil {
			return fmt.Errorf("resolve container %s image: %w", c.ID, err)
		}
		if inspected.ID == "" {
			return fmt.Errorf("container %s image has no identity", c.ID)
		}
		referenced[inspected.ID] = true
	}
	seen := make(map[string]bool)
	for _, item := range listed.Items {
		inspected, err := docker.ImageInspect(ctx, item.ID)
		if errdefs.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect cleanup image %s: %w", item.ID, err)
		}
		id := inspected.ID
		if id == "" {
			return fmt.Errorf("cleanup image %s has no identity", item.ID)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		r.mu.Lock()
		if r.imageLastUsed == nil {
			r.imageLastUsed = make(map[string]time.Time)
		}
		lastUsed, known := r.imageLastUsed[id]
		if !known || referenced[id] {
			// Unknown history, including a node restart, gets a full grace period.
			lastUsed = now
			r.imageLastUsed[id] = lastUsed
		}
		r.mu.Unlock()
		if !lastUsed.Before(now.Add(-age)) {
			continue
		}
		// Docker refuses deletion by ID when multiple references exist. Remove
		// each verified alias without force; disable implicit ancestor deletion.
		refs := append(append([]string{}, inspected.RepoTags...), inspected.RepoDigests...)
		refs = append(refs, id)
		for _, ref := range refs {
			if ref == "" || ref == "<none>:<none>" || ref == "<none>@<none>" {
				continue
			}
			current, err := docker.ImageInspect(ctx, ref)
			if errdefs.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("recheck cleanup image %s: %w", ref, err)
			}
			if current.ID != id {
				continue
			}
			_, err = docker.ImageRemove(ctx, ref, client.ImageRemoveOptions{})
			if errdefs.IsConflict(err) {
				break
			}
			if err != nil && !errdefs.IsNotFound(err) {
				return fmt.Errorf("remove unused image %s: %w", ref, err)
			}
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range r.imageLastUsed {
		if !seen[id] {
			delete(r.imageLastUsed, id)
		}
	}
	return nil
}
