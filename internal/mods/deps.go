package mods

import (
	"context"
	"time"

	gitapi "github.com/iw2rmb/ploy/api/git"
)

// ArtifactUploader abstracts artifact upload operations so tests can inject fakes.
type ArtifactUploader interface {
	UploadFile(ctx context.Context, baseURL, key, srcPath, contentType string) error
	UploadJSON(ctx context.Context, baseURL, key string, body []byte) error
}

// BuilderSubmitter abstracts job validation/submission flows for builders.
type BuilderSubmitter interface {
	Validate(ctx context.Context, hclPath string) error
	Submit(ctx context.Context, hclPath string, timeout time.Duration) error
}

// GitPushOperation exposes observable git push behaviour.
type GitPushOperation interface {
	Events() <-chan gitapi.Event
	Err() error
}

// GitPusher performs asynchronous git pushes and returns an operation handle.
type GitPusher interface {
	PushBranchAsync(ctx context.Context, repoPath, remoteURL, branch string) GitPushOperation
}
