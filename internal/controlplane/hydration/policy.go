package hydration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ErrPolicyVersionConflict indicates the stored policy was modified concurrently.
var ErrPolicyVersionConflict = errors.New("hydration: policy version conflict")

// PolicyStore coordinates persistence for global hydration policies.
type PolicyStore struct {
	client *clientv3.Client
	prefix string
	clock  func() time.Time
}

// PolicyStoreOptions configures policy storage behaviour.
type PolicyStoreOptions struct {
	Prefix string
	Clock  func() time.Time
}

// GlobalPolicy captures retention limits and scope matchers enforced cluster wide.
type GlobalPolicy struct {
	ID          string
	Description string
	Scope       PolicyScope
	Window      QuotaWindow
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Usage       PolicyUsage
}

// PolicyScope narrows policy application to matching repositories or owners.
type PolicyScope struct {
	RepoPrefixes []string
	Owners       []string
}

// QuotaWindow defines soft/hard ceilings for policy governed resources.
type QuotaWindow struct {
	PinnedBytes LimitBytes
	Snapshots   LimitCount
	Replicas    LimitCount
}

// LimitBytes encodes byte based ceilings with optional soft limits.
type LimitBytes struct {
	Soft int64
	Hard int64
}

// LimitCount encodes count based ceilings with optional soft limits.
type LimitCount struct {
	Soft int
	Hard int
}

// PolicyUsage summarises current resource consumption for a policy scope.
type PolicyUsage struct {
	PolicyID           string
	PinnedBytes        int64
	SnapshotCount      int
	ReplicaCount       int
	ActiveFingerprints []string
	UpdatedAt          time.Time
}

// NewPolicyStore constructs a policy store backed by etcd.
func NewPolicyStore(client *clientv3.Client, opts PolicyStoreOptions) (*PolicyStore, error) {
	if client == nil {
		return nil, errors.New("hydration: etcd client required")
	}
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = "hydration/policies/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &PolicyStore{
		client: client,
		prefix: prefix,
		clock:  clock,
	}, nil
}

// MatchSnapshot resolves the policy governing the provided snapshot entry.
func (s *PolicyStore) MatchSnapshot(ctx context.Context, entry SnapshotEntry) (GlobalPolicy, bool, error) {
	var zero GlobalPolicy
	if s == nil {
		return zero, false, errors.New("hydration: policy store not configured")
	}
	policies, err := s.ListPolicies(ctx)
	if err != nil {
		return zero, false, err
	}
	policy := matchPolicyForEntry(policies, entry)
	if policy == nil {
		return zero, false, nil
	}
	copy := *policy
	return copy, true, nil
}

// ListPolicies returns all stored policies ordered by identifier.
func (s *PolicyStore) ListPolicies(ctx context.Context) ([]GlobalPolicy, error) {
	if s == nil {
		return nil, errors.New("hydration: policy store not configured")
	}
	resp, err := s.client.Get(ctx, s.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("hydration: list policies: %w", err)
	}
	policies := make([]GlobalPolicy, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var doc policyDocument
		if err := json.Unmarshal(kv.Value, &doc); err != nil {
			return nil, fmt.Errorf("hydration: decode policy %s: %w", string(kv.Key), err)
		}
		policy, err := doc.toPolicy()
		if err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].ID < policies[j].ID
	})
	return policies, nil
}

// SavePolicy upserts a policy definition, enforcing optimistic concurrency on updates.
func (s *PolicyStore) SavePolicy(ctx context.Context, policy GlobalPolicy) (GlobalPolicy, error) {
	var zero GlobalPolicy
	if s == nil {
		return zero, errors.New("hydration: policy store not configured")
	}
	if err := validatePolicy(policy); err != nil {
		return zero, err
	}
	id := strings.TrimSpace(policy.ID)
	key := s.policyKey(id)
	now := s.clock().UTC()

	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return zero, fmt.Errorf("hydration: fetch policy %s: %w", id, err)
	}

	if len(resp.Kvs) == 0 {
		doc := newPolicyDocument(policy)
		doc.ID = id
		doc.CreatedAt = encodeTime(now)
		doc.UpdatedAt = doc.CreatedAt
		doc.Version = 1
		payload, err := json.Marshal(doc)
		if err != nil {
			return zero, fmt.Errorf("hydration: marshal policy: %w", err)
		}
		if _, err := s.client.Put(ctx, key, string(payload)); err != nil {
			return zero, fmt.Errorf("hydration: persist policy: %w", err)
		}
		return doc.toPolicy()
	}

	var stored policyDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &stored); err != nil {
		return zero, fmt.Errorf("hydration: decode policy %s: %w", id, err)
	}
	if policy.Version == 0 {
		return zero, ErrPolicyVersionConflict
	}
	if policy.Version != stored.Version {
		return zero, ErrPolicyVersionConflict
	}

	doc := newPolicyDocument(policy)
	doc.ID = id
	doc.CreatedAt = stored.CreatedAt
	doc.Version = stored.Version + 1
	doc.Usage = stored.Usage
	doc.UpdatedAt = encodeTime(now)

	payload, err := json.Marshal(doc)
	if err != nil {
		return zero, fmt.Errorf("hydration: marshal policy: %w", err)
	}

	txn := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", resp.Kvs[0].ModRevision)).
		Then(clientv3.OpPut(key, string(payload)))

	result, err := txn.Commit()
	if err != nil {
		return zero, fmt.Errorf("hydration: update policy: %w", err)
	}
	if !result.Succeeded {
		return zero, ErrPolicyVersionConflict
	}
	return doc.toPolicy()
}

// RecordUsage replaces the stored usage snapshot for the given policy.
func (s *PolicyStore) RecordUsage(ctx context.Context, policyID string, usage PolicyUsage) (GlobalPolicy, error) {
	var zero GlobalPolicy
	if s == nil {
		return zero, errors.New("hydration: policy store not configured")
	}
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return zero, errors.New("hydration: policy id required")
	}
	key := s.policyKey(policyID)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return zero, fmt.Errorf("hydration: fetch policy %s: %w", policyID, err)
	}
	if len(resp.Kvs) == 0 {
		return zero, fmt.Errorf("hydration: policy %s missing", policyID)
	}

	var doc policyDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return zero, fmt.Errorf("hydration: decode policy %s: %w", policyID, err)
	}

	now := s.clock().UTC()
	doc.Usage = usageDocumentFrom(usage, now)
	doc.Version++
	doc.UpdatedAt = encodeTime(now)

	payload, err := json.Marshal(doc)
	if err != nil {
		return zero, fmt.Errorf("hydration: marshal usage: %w", err)
	}

	txn := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", resp.Kvs[0].ModRevision)).
		Then(clientv3.OpPut(key, string(payload)))

	result, err := txn.Commit()
	if err != nil {
		return zero, fmt.Errorf("hydration: update usage: %w", err)
	}
	if !result.Succeeded {
		return zero, ErrPolicyVersionConflict
	}
	return doc.toPolicy()
}

func (s *PolicyStore) policyKey(id string) string {
	return path.Join(s.prefix, id)
}

func validatePolicy(policy GlobalPolicy) error {
	id := strings.TrimSpace(policy.ID)
	if id == "" {
		return errors.New("hydration: policy id required")
	}
	if err := policy.Window.validate(); err != nil {
		return err
	}
	return nil
}

func (w QuotaWindow) validate() error {
	if err := w.PinnedBytes.validate("pinned bytes"); err != nil {
		return err
	}
	if err := w.Snapshots.validate("snapshots"); err != nil {
		return err
	}
	if err := w.Replicas.validate("replicas"); err != nil {
		return err
	}
	return nil
}

func (l LimitBytes) validate(field string) error {
	if l.Hard < 0 {
		return fmt.Errorf("hydration: %s hard limit must be non-negative", field)
	}
	if l.Soft < 0 {
		return fmt.Errorf("hydration: %s soft limit must be non-negative", field)
	}
	if l.Soft != 0 && l.Hard != 0 && l.Soft > l.Hard {
		return fmt.Errorf("hydration: %s soft limit exceeds hard limit", field)
	}
	return nil
}

func (l LimitCount) validate(field string) error {
	if l.Hard < 0 {
		return fmt.Errorf("hydration: %s hard limit must be non-negative", field)
	}
	if l.Soft < 0 {
		return fmt.Errorf("hydration: %s soft limit must be non-negative", field)
	}
	if l.Soft != 0 && l.Hard != 0 && l.Soft > l.Hard {
		return fmt.Errorf("hydration: %s soft limit exceeds hard limit", field)
	}
	return nil
}

func newPolicyDocument(policy GlobalPolicy) policyDocument {
	scope := normalizeScope(policy.Scope)
	window := windowDocumentFrom(policy.Window)
	return policyDocument{
		ID:          strings.TrimSpace(policy.ID),
		Description: strings.TrimSpace(policy.Description),
		Scope:       scope,
		Window:      window,
		Usage:       policyUsageDocument{},
	}
}

func normalizeScope(scope PolicyScope) policyScopeDocument {
	doc := policyScopeDocument{}
	if len(scope.RepoPrefixes) > 0 {
		doc.RepoPrefixes = normalizeStrings(scope.RepoPrefixes)
	}
	if len(scope.Owners) > 0 {
		doc.Owners = normalizeStrings(scope.Owners)
	}
	return doc
}

func normalizeStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func windowDocumentFrom(window QuotaWindow) quotaWindowDocument {
	return quotaWindowDocument{
		PinnedBytes: limitBytesDocument{
			Soft: window.PinnedBytes.Soft,
			Hard: window.PinnedBytes.Hard,
		},
		Snapshots: limitCountDocument{
			Soft: window.Snapshots.Soft,
			Hard: window.Snapshots.Hard,
		},
		Replicas: limitCountDocument{
			Soft: window.Replicas.Soft,
			Hard: window.Replicas.Hard,
		},
	}
}

func usageDocumentFrom(usage PolicyUsage, now time.Time) policyUsageDocument {
	doc := policyUsageDocument{
		PinnedBytes:   usage.PinnedBytes,
		SnapshotCount: usage.SnapshotCount,
		ReplicaCount:  usage.ReplicaCount,
		UpdatedAt:     encodeTime(now),
	}
	if len(usage.ActiveFingerprints) > 0 {
		doc.ActiveFingerprints = normalizeStrings(usage.ActiveFingerprints)
	}
	return doc
}

type policyDocument struct {
	ID          string              `json:"id"`
	Description string              `json:"description,omitempty"`
	Scope       policyScopeDocument `json:"scope"`
	Window      quotaWindowDocument `json:"window"`
	Usage       policyUsageDocument `json:"usage"`
	Version     int64               `json:"version"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
}

type policyScopeDocument struct {
	RepoPrefixes []string `json:"repo_prefixes,omitempty"`
	Owners       []string `json:"owners,omitempty"`
}

type quotaWindowDocument struct {
	PinnedBytes limitBytesDocument `json:"pinned_bytes"`
	Snapshots   limitCountDocument `json:"snapshots"`
	Replicas    limitCountDocument `json:"replicas"`
}

type limitBytesDocument struct {
	Soft int64 `json:"soft,omitempty"`
	Hard int64 `json:"hard,omitempty"`
}

type limitCountDocument struct {
	Soft int `json:"soft,omitempty"`
	Hard int `json:"hard,omitempty"`
}

type policyUsageDocument struct {
	PinnedBytes        int64    `json:"pinned_bytes"`
	SnapshotCount      int      `json:"snapshot_count"`
	ReplicaCount       int      `json:"replica_count"`
	ActiveFingerprints []string `json:"active_fingerprints,omitempty"`
	UpdatedAt          string   `json:"updated_at"`
}

func (d policyDocument) toPolicy() (GlobalPolicy, error) {
	created, err := parseTime(d.CreatedAt)
	if err != nil {
		return GlobalPolicy{}, fmt.Errorf("hydration: parse policy created_at: %w", err)
	}
	updated, err := parseTime(d.UpdatedAt)
	if err != nil {
		return GlobalPolicy{}, fmt.Errorf("hydration: parse policy updated_at: %w", err)
	}
	usage, err := d.Usage.toUsage(d.ID)
	if err != nil {
		return GlobalPolicy{}, err
	}
	return GlobalPolicy{
		ID:          d.ID,
		Description: d.Description,
		Scope:       d.Scope.toScope(),
		Window:      d.Window.toWindow(),
		Version:     d.Version,
		CreatedAt:   created,
		UpdatedAt:   updated,
		Usage:       usage,
	}, nil
}

func (d policyScopeDocument) toScope() PolicyScope {
	scope := PolicyScope{}
	if len(d.RepoPrefixes) > 0 {
		scope.RepoPrefixes = append([]string(nil), d.RepoPrefixes...)
	}
	if len(d.Owners) > 0 {
		scope.Owners = append([]string(nil), d.Owners...)
	}
	return scope
}

func (d quotaWindowDocument) toWindow() QuotaWindow {
	return QuotaWindow{
		PinnedBytes: LimitBytes{Soft: d.PinnedBytes.Soft, Hard: d.PinnedBytes.Hard},
		Snapshots:   LimitCount{Soft: d.Snapshots.Soft, Hard: d.Snapshots.Hard},
		Replicas:    LimitCount{Soft: d.Replicas.Soft, Hard: d.Replicas.Hard},
	}
}

func (d policyUsageDocument) toUsage(policyID string) (PolicyUsage, error) {
	updated, err := parseTime(d.UpdatedAt)
	if err != nil {
		return PolicyUsage{}, fmt.Errorf("hydration: parse policy usage updated_at: %w", err)
	}
	usage := PolicyUsage{
		PolicyID:      strings.TrimSpace(policyID),
		PinnedBytes:   d.PinnedBytes,
		SnapshotCount: d.SnapshotCount,
		ReplicaCount:  d.ReplicaCount,
		UpdatedAt:     updated,
	}
	if len(d.ActiveFingerprints) > 0 {
		usage.ActiveFingerprints = append([]string(nil), d.ActiveFingerprints...)
	}
	return usage, nil
}

func matchPolicyForEntry(policies []GlobalPolicy, snapshot SnapshotEntry) *GlobalPolicy {
	repo := strings.ToLower(strings.TrimSpace(snapshot.RepoURL))
	for i := range policies {
		scope := policies[i].Scope
		if len(scope.RepoPrefixes) == 0 && len(scope.Owners) == 0 {
			return &policies[i]
		}
		if matchRepoPrefix(scope.RepoPrefixes, repo) {
			return &policies[i]
		}
	}
	return nil
}

func matchRepoPrefix(prefixes []string, repo string) bool {
	if len(prefixes) == 0 {
		return false
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(repo, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}
