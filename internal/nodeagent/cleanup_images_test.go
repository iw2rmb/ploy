package nodeagent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
)

// Creation age never overrides recent use; unknown history receives a full grace period.
func TestCleanupImagesUsesLastUse(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name       string
		used       time.Time
		created    time.Time
		referenced bool
		remove     bool
	}{
		{"old image recently used", now.Add(-time.Hour), now.Add(-365 * 24 * time.Hour), false, false},
		{"recent image unused", now.Add(-25 * time.Hour), now, false, true},
		{"unknown history", time.Time{}, now.Add(-365 * 24 * time.Hour), false, false},
		{"boundary", now.Add(-24 * time.Hour), now, false, false},
		{"container reference", now.Add(-25 * time.Hour), now, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &runController{imageLastUsed: map[string]time.Time{}}
			if !tc.used.IsZero() {
				r.imageLastUsed["image"] = tc.used
			}
			f := &cleanupDockerFixture{imageItems: []image.Summary{{ID: "image", Created: tc.created.Unix()}}}
			var containers []container.InspectResponse
			if tc.referenced {
				containers = []container.InspectResponse{{ID: "test", Image: "image"}}
			}
			if err := r.pruneCleanupImages(context.Background(), f, containers, now, 24*time.Hour); err != nil {
				t.Fatal(err)
			}
			if (len(f.imageRemoved) != 0) != tc.remove {
				t.Fatalf("removed %v", f.imageRemoved)
			}
			for _, opts := range f.imageOptions {
				if opts.Force || opts.PruneChildren {
					t.Fatalf("unsafe remove options %+v", opts)
				}
			}
		})
	}
}

// Observations do not extend idle-image retention, but use and unknown-history resets do.
func TestCleanupImagesGraceAndInvalidation(t *testing.T) {
	now := time.Now()
	r := &runController{}
	f := &cleanupDockerFixture{imageItems: []image.Summary{{ID: "image"}}}
	for _, at := range []time.Time{now, now.Add(23 * time.Hour), now.Add(25 * time.Hour)} {
		if err := r.pruneCleanupImages(context.Background(), f, nil, at, 24*time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(f.imageRemoved, []string{"image"}) {
		t.Fatalf("removed %v", f.imageRemoved)
	}
	f.imageRemoved = nil
	r.imageLastUsed["image"] = now.Add(-48 * time.Hour)
	r.recordImageUse("image")
	if err := r.pruneCleanupImages(context.Background(), f, nil, now.Add(time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(f.imageRemoved) != 0 {
		t.Fatal("recently used image removed")
	}
	r.imageLastUsed["image"] = now.Add(-48 * time.Hour)
	r.invalidateImageUse()
	if err := r.pruneCleanupImages(context.Background(), f, nil, now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(f.imageRemoved) != 0 {
		t.Fatal("invalidated history authorized deletion")
	}
}

// Multiple aliases share one canonical age; a changed alias belongs to its new image.
func TestCleanupImagesRemovesOnlyExpiredAliases(t *testing.T) {
	now := time.Now()
	r := &runController{imageLastUsed: map[string]time.Time{"image": now.Add(-25 * time.Hour)}}
	f := &cleanupDockerFixture{imageItems: []image.Summary{{ID: "image"}}}
	f.imageInspect = func(ref string) (client.ImageInspectResult, error) {
		if ref == "changed:latest" {
			return client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "replacement"}}, nil
		}
		return client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: "image", RepoTags: []string{"repo:a", "repo:b", "changed:latest"}, RepoDigests: []string{"repo@sha256:digest"}}}, nil
	}
	if err := r.pruneCleanupImages(context.Background(), f, nil, now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.imageRemoved, []string{"repo:a", "repo:b", "repo@sha256:digest", "image"}) {
		t.Fatalf("removed %v", f.imageRemoved)
	}
}

// Failed container image resolution cannot prove that an expired image is unused.
func TestCleanupImagesPreservesUnresolvedContainer(t *testing.T) {
	now := time.Now()
	r := &runController{imageLastUsed: map[string]time.Time{"image": now.Add(-25 * time.Hour)}}
	f := &cleanupDockerFixture{imageItems: []image.Summary{{ID: "image"}}, imageInspect: func(string) (client.ImageInspectResult, error) {
		return client.ImageInspectResult{}, errors.New("unavailable")
	}}
	if err := r.pruneCleanupImages(context.Background(), f, []container.InspectResponse{{Image: "image"}}, now, 24*time.Hour); err == nil {
		t.Fatal("accepted unavailable inspection")
	}
	if len(f.imageRemoved) != 0 {
		t.Fatal("removed unresolved image")
	}
}
