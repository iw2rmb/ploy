package main

import (
	"context"
	"time"

	gitlabcfg "github.com/iw2rmb/ploy/internal/config/gitlab"
)

const (
	controlPlaneURLEnv   = "PLOY_CONTROL_PLANE_URL"
	defaultGitlabTimeout = 10 * time.Second
)

type gitlabStore interface {
	Load(context.Context) (gitlabcfg.Config, int64, error)
	Save(context.Context, gitlabcfg.Config) (int64, error)
}

type gitlabStoreCloser interface {
	gitlabStore
	Close() error
}

type gitlabSignerClient interface {
	Status(ctx context.Context, req gitlabSignerStatusRequest) (gitlabSignerStatus, error)
	RotateSecret(ctx context.Context, req gitlabRotateSecretRequest) (gitlabRotateSecretResult, error)
}

type gitlabSignerStatusRequest struct {
	Secret string
}

type gitlabSignerStatus struct {
	FeedRevision int64
	Secrets      []gitlabSignerSecretStatus
}

type gitlabSignerSecretStatus struct {
	Name      string
	Revision  int64
	RotatedAt time.Time
	Scopes    []string
	Audit     gitlabSignerAudit
}

type gitlabSignerAudit struct {
	LastRotation time.Time
	Revocations  []gitlabSignerRevocation
	Failures     []gitlabSignerFailure
}

type gitlabSignerRevocation struct {
	NodeID    string
	TokenID   string
	Timestamp time.Time
}

type gitlabSignerFailure struct {
	NodeID    string
	TokenID   string
	Timestamp time.Time
	Error     string
}

type gitlabRotateSecretRequest struct {
	Secret string
	APIKey string
	Scopes []string
}

type gitlabRotateSecretResult struct {
	Secret    string
	Revision  int64
	UpdatedAt time.Time
	Scopes    []string
}
