package mods

import (
	"context"

	gitapi "github.com/iw2rmb/ploy/api/git"
)

// HTTPArtifactUploader implements ArtifactUploader using in-process HTTP helpers.
type HTTPArtifactUploader struct{}

// NewHTTPArtifactUploader constructs an HTTPArtifactUploader.
func NewHTTPArtifactUploader() *HTTPArtifactUploader { return &HTTPArtifactUploader{} }

// UploadFile uploads the file located at srcPath to the artifacts namespace.
func (HTTPArtifactUploader) UploadFile(ctx context.Context, baseURL, key, srcPath, contentType string) error {
	_ = ctx
	return putFileFn(baseURL, key, srcPath, contentType)
}

// UploadJSON uploads JSON bytes to the artifacts namespace.
func (HTTPArtifactUploader) UploadJSON(ctx context.Context, baseURL, key string, body []byte) error {
	_ = ctx
	return putJSONFn(baseURL, key, body)
}

// gitPushOperation wraps gitapi.Operation to implement GitPushOperation.
type gitPushOperation struct{ op *gitapi.Operation }

func (o *gitPushOperation) Events() <-chan gitapi.Event {
	if o == nil || o.op == nil {
		return nil
	}
	return o.op.Events()
}

func (o *gitPushOperation) Err() error {
	if o == nil || o.op == nil {
		return nil
	}
	return o.op.Err()
}

// gitOpsPusher adapts GitOperationsInterface into GitPusher.
type gitOpsPusher struct{ git GitOperationsInterface }

func newGitOpsPusher(git GitOperationsInterface) GitPusher {
	if git == nil {
		return nil
	}
	return &gitOpsPusher{git: git}
}

func (p *gitOpsPusher) PushBranchAsync(ctx context.Context, repoPath, remoteURL, branch string) GitPushOperation {
	if p == nil || p.git == nil {
		return &gitPushOperation{}
	}
	return &gitPushOperation{op: p.git.PushBranchAsync(ctx, repoPath, remoteURL, branch)}
}
