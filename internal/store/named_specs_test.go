package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestNamedSpecs_CreateLookupAndUniqueIndex(t *testing.T) {
	ctx, db := newTestStore(t)
	now := time.Now().UTC()
	source := mustNamedSpecSourceJSON(t, "github.com", "acme/service")
	params := CreateNamedSpecParams{
		ID:                types.NewSpecID(),
		Name:              "upgrade-java",
		Description:       "Upgrade Java",
		Source:            source,
		Sha:               "0123456789abcdef0123456789abcdef01234567",
		SourceCommittedAt: pgtype.Timestamptz{Time: now, Valid: true},
		Spec:              []byte(`{"apiVersion":"ploy.mig/v1alpha1","name":"upgrade-java","steps":[{"image":"img"}]}`),
	}

	created, err := db.CreateNamedSpec(ctx, params)
	if err != nil {
		t.Fatalf("CreateNamedSpec() failed: %v", err)
	}
	if created.Description != params.Description || created.Sha != params.Sha {
		t.Fatalf("created row mismatch: got=%+v", created)
	}

	fetched, err := db.GetNamedSpecByNameSourceSHA(ctx, GetNamedSpecByNameSourceSHAParams{
		Name: "upgrade-java", Domain: "github.com", Repo: "acme/service", Sha: params.Sha,
	})
	if err != nil {
		t.Fatalf("GetNamedSpecByNameSourceSHA() failed: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("lookup ID=%q, want %q", fetched.ID, created.ID)
	}

	params.ID = types.NewSpecID()
	_, err = db.CreateNamedSpec(ctx, params)
	if err == nil {
		t.Fatal("expected duplicate named source sha insert to fail")
	}
	assertUniqueViolation(t, err)
}

func TestNamedSpecs_LatestListAndResolve(t *testing.T) {
	ctx, db := newTestStore(t)
	base := time.Now().UTC()
	rows := []struct {
		name   string
		domain string
		repo   string
		sha    string
		at     time.Time
	}{
		{name: "upgrade-java", domain: "github.com", repo: "acme/service", sha: "1111111111111111111111111111111111111111", at: base.Add(-3 * time.Hour)},
		{name: "upgrade-java", domain: "github.com", repo: "acme/service", sha: "2222222222222222222222222222222222222222", at: base.Add(-1 * time.Hour)},
		{name: "upgrade-java", domain: "gitlab.example.com", repo: "acme/service", sha: "3333333333333333333333333333333333333333", at: base.Add(-2 * time.Hour)},
		{name: "modernize", domain: "github.com", repo: "acme/api", sha: "4444444444444444444444444444444444444444", at: base},
	}
	for _, row := range rows {
		_, err := db.CreateNamedSpec(ctx, CreateNamedSpecParams{
			ID:                types.NewSpecID(),
			Name:              row.name,
			Source:            mustNamedSpecSourceJSON(t, row.domain, row.repo),
			Sha:               row.sha,
			SourceCommittedAt: pgtype.Timestamptz{Time: row.at, Valid: true},
			Spec:              []byte(`{"apiVersion":"ploy.mig/v1alpha1","name":"` + row.name + `","steps":[{"image":"img"}]}`),
		})
		if err != nil {
			t.Fatalf("CreateNamedSpec(%s/%s:%s@%s) failed: %v", row.domain, row.repo, row.name, row.sha, err)
		}
	}

	latest, err := db.ListLatestNamedSpecs(ctx, ListLatestNamedSpecsParams{Limit: 10, Offset: 0, Archived: false})
	if err != nil {
		t.Fatalf("ListLatestNamedSpecs() failed: %v", err)
	}
	if len(latest) != 3 {
		t.Fatalf("latest rows=%d, want 3", len(latest))
	}
	assertLatestNamedSpecRow(t, latest, "github.com", "acme/service", "upgrade-java", "2222222222222222222222222222222222222222")

	tests := []struct {
		name      string
		query     func() ([]string, error)
		wantCount int
		wantSHA   string
	}{
		{
			name: "by name returns latest per source",
			query: func() ([]string, error) {
				rows, err := db.ResolveLatestNamedSpecByName(ctx, ResolveLatestNamedSpecByNameParams{Name: "upgrade-java", Archived: false})
				return resolveNameSHAs(rows), err
			},
			wantCount: 2,
		},
		{
			name: "by repo and name matches any domain",
			query: func() ([]string, error) {
				rows, err := db.ResolveLatestNamedSpecByRepoName(ctx, ResolveLatestNamedSpecByRepoNameParams{Name: "upgrade-java", Repo: "acme/service", Archived: false})
				return resolveRepoNameSHAs(rows), err
			},
			wantCount: 2,
		},
		{
			name: "by domain repo and name is exact",
			query: func() ([]string, error) {
				rows, err := db.ResolveLatestNamedSpecByDomainRepoName(ctx, ResolveLatestNamedSpecByDomainRepoNameParams{Name: "upgrade-java", Domain: "github.com", Repo: "acme/service", Archived: false})
				return resolveDomainRepoNameSHAs(rows), err
			},
			wantCount: 1,
			wantSHA:   "2222222222222222222222222222222222222222",
		},
		{
			name: "missing returns empty",
			query: func() ([]string, error) {
				rows, err := db.ResolveLatestNamedSpecByName(ctx, ResolveLatestNamedSpecByNameParams{Name: "missing", Archived: false})
				return resolveNameSHAs(rows), err
			},
			wantCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shas, err := tt.query()
			if err != nil {
				t.Fatalf("resolve failed: %v", err)
			}
			if len(shas) != tt.wantCount {
				t.Fatalf("count=%d, want %d, shas=%v", len(shas), tt.wantCount, shas)
			}
			if tt.wantSHA != "" && shas[0] != tt.wantSHA {
				t.Fatalf("sha=%q, want %q", shas[0], tt.wantSHA)
			}
		})
	}
}

func TestNamedSpecs_ArchiveFilterVersionResolveAndUpdatedBy(t *testing.T) {
	ctx, db := newTestStore(t)
	base := time.Now().UTC()
	creator := "creator"
	updater := "archiver"
	activeSHA := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	archivedSHA := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	ambiguousSHA1 := "abcdef1200000000000000000000000000000000"
	ambiguousSHA2 := "abcdef12ffffffffffffffffffffffffffffffff"

	for _, row := range []struct {
		name   string
		domain string
		repo   string
		sha    string
		at     time.Time
	}{
		{name: "upgrade-java", domain: "github.com", repo: "acme/service", sha: activeSHA, at: base.Add(-2 * time.Hour)},
		{name: "upgrade-java", domain: "github.com", repo: "acme/service", sha: archivedSHA, at: base.Add(-1 * time.Hour)},
		{name: "upgrade-java", domain: "gitlab.example.com", repo: "acme/service", sha: ambiguousSHA1, at: base},
		{name: "upgrade-java", domain: "gitea.example.com", repo: "acme/service", sha: ambiguousSHA2, at: base},
	} {
		created, err := db.CreateNamedSpec(ctx, CreateNamedSpecParams{
			ID:                types.NewSpecID(),
			Name:              row.name,
			Source:            mustNamedSpecSourceJSON(t, row.domain, row.repo),
			Sha:               row.sha,
			SourceCommittedAt: pgtype.Timestamptz{Time: row.at, Valid: true},
			Spec:              []byte(`{"apiVersion":"ploy.mig/v1alpha1","name":"` + row.name + `","steps":[{"image":"img"}]}`),
			CreatedBy:         &creator,
		})
		if err != nil {
			t.Fatalf("CreateNamedSpec(%s) failed: %v", row.sha, err)
		}
		if created.CreatedBy == nil || *created.CreatedBy != creator || created.UpdatedBy == nil || *created.UpdatedBy != creator {
			t.Fatalf("created identity = created_by:%v updated_by:%v, want %q", created.CreatedBy, created.UpdatedBy, creator)
		}
		if row.sha == archivedSHA {
			updated, err := db.UpdateNamedSpecArchiveState(ctx, UpdateNamedSpecArchiveStateParams{ID: created.ID.String(), Archived: true, UpdatedBy: &updater})
			if err != nil {
				t.Fatalf("UpdateNamedSpecArchiveState() failed: %v", err)
			}
			if !updated.ArchivedAt.Valid || updated.UpdatedBy == nil || *updated.UpdatedBy != updater {
				t.Fatalf("archive state = archived_at:%v updated_by:%v", updated.ArchivedAt, updated.UpdatedBy)
			}
		}
	}

	tests := []struct {
		name      string
		query     func() ([]string, error)
		wantCount int
		wantSHA   string
	}{
		{
			name: "active list picks latest active before grouping",
			query: func() ([]string, error) {
				rows, err := db.ListLatestNamedSpecs(ctx, ListLatestNamedSpecsParams{Limit: 10, Archived: false})
				return listNamedSHAsForSource(rows, "github.com", "acme/service"), err
			},
			wantCount: 1,
			wantSHA:   activeSHA,
		},
		{
			name: "archived list picks latest archived",
			query: func() ([]string, error) {
				rows, err := db.ListLatestNamedSpecs(ctx, ListLatestNamedSpecsParams{Limit: 10, Archived: true})
				return listNamedSHAsForSource(rows, "github.com", "acme/service"), err
			},
			wantCount: 1,
			wantSHA:   archivedSHA,
		},
		{
			name: "active version prefix ignores archived row",
			query: func() ([]string, error) {
				rows, err := db.ResolveNamedSpecVersionByDomainRepoName(ctx, ResolveNamedSpecVersionByDomainRepoNameParams{Name: "upgrade-java", Domain: "github.com", Repo: "acme/service", ShaPrefix: activeSHA[:8], Archived: false})
				return specSHAs(rows), err
			},
			wantCount: 1,
			wantSHA:   activeSHA,
		},
		{
			name: "archived version prefix resolves archived row",
			query: func() ([]string, error) {
				rows, err := db.ResolveNamedSpecVersionByDomainRepoName(ctx, ResolveNamedSpecVersionByDomainRepoNameParams{Name: "upgrade-java", Domain: "github.com", Repo: "acme/service", ShaPrefix: archivedSHA[:8], Archived: true})
				return specSHAs(rows), err
			},
			wantCount: 1,
			wantSHA:   archivedSHA,
		},
		{
			name: "ambiguous sha prefix returns multiple matches",
			query: func() ([]string, error) {
				rows, err := db.ResolveNamedSpecVersionByName(ctx, ResolveNamedSpecVersionByNameParams{Name: "upgrade-java", ShaPrefix: "abcdef12", Archived: false})
				return specSHAs(rows), err
			},
			wantCount: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shas, err := tt.query()
			if err != nil {
				t.Fatalf("query failed: %v", err)
			}
			if len(shas) != tt.wantCount {
				t.Fatalf("count=%d, want %d, shas=%v", len(shas), tt.wantCount, shas)
			}
			if tt.wantSHA != "" && shas[0] != tt.wantSHA {
				t.Fatalf("sha=%q, want %q", shas[0], tt.wantSHA)
			}
		})
	}
}

func TestNamedSpecs_NamedConstraints(t *testing.T) {
	ctx, db := newTestStore(t)
	tests := []struct {
		name string
		arg  CreateNamedSpecParams
	}{
		{name: "bad sha", arg: CreateNamedSpecParams{Name: "upgrade-java", Source: mustNamedSpecSourceJSON(t, "github.com", "acme/service"), Sha: "ABC", SourceCommittedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, Spec: []byte(`{}`)}},
		{name: "missing source", arg: CreateNamedSpecParams{Name: "upgrade-java", Source: []byte(`{}`), Sha: "5555555555555555555555555555555555555555", SourceCommittedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, Spec: []byte(`{}`)}},
		{name: "missing committed at", arg: CreateNamedSpecParams{Name: "upgrade-java", Source: mustNamedSpecSourceJSON(t, "github.com", "acme/service"), Sha: "6666666666666666666666666666666666666666", Spec: []byte(`{}`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.arg.ID = types.NewSpecID()
			_, err := db.CreateNamedSpec(ctx, tt.arg)
			if err == nil {
				t.Fatal("expected constraint violation")
			}
		})
	}

	_, err := db.CreateSpec(ctx, CreateSpecParams{ID: types.NewSpecID(), Name: "label-only", Spec: []byte(`{}`)})
	if err != nil {
		t.Fatalf("CreateSpec() with name and empty sha should remain valid: %v", err)
	}
	_, err = db.GetNamedSpecByNameSourceSHA(ctx, GetNamedSpecByNameSourceSHAParams{Name: "missing", Domain: "github.com", Repo: "acme/service", Sha: "abcdefabcdefabcdefabcdefabcdefabcdefabcd"})
	if err != pgx.ErrNoRows {
		t.Fatalf("missing named lookup err=%v, want pgx.ErrNoRows", err)
	}
}

func mustNamedSpecSourceJSON(t *testing.T, domain, repo string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"domain": domain, "repo": repo})
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}
	return raw
}

func assertLatestNamedSpecRow(t *testing.T, rows []ListLatestNamedSpecsRow, domain, repo, name, sha string) {
	t.Helper()
	for _, row := range rows {
		var source struct {
			Domain string `json:"domain"`
			Repo   string `json:"repo"`
		}
		if err := json.Unmarshal(row.Source, &source); err != nil {
			t.Fatalf("unmarshal row source: %v", err)
		}
		if source.Domain == domain && source.Repo == repo && row.Name == name {
			if row.Sha != sha {
				t.Fatalf("latest sha for %s/%s:%s=%s, want %s", domain, repo, name, row.Sha, sha)
			}
			return
		}
	}
	t.Fatalf("latest rows missing %s/%s:%s", domain, repo, name)
}

func resolveNameSHAs(rows []ResolveLatestNamedSpecByNameRow) []string {
	shas := make([]string, 0, len(rows))
	for _, row := range rows {
		shas = append(shas, row.Sha)
	}
	return shas
}

func resolveRepoNameSHAs(rows []ResolveLatestNamedSpecByRepoNameRow) []string {
	shas := make([]string, 0, len(rows))
	for _, row := range rows {
		shas = append(shas, row.Sha)
	}
	return shas
}

func resolveDomainRepoNameSHAs(rows []ResolveLatestNamedSpecByDomainRepoNameRow) []string {
	shas := make([]string, 0, len(rows))
	for _, row := range rows {
		shas = append(shas, row.Sha)
	}
	return shas
}

func specSHAs(rows []Spec) []string {
	shas := make([]string, 0, len(rows))
	for _, row := range rows {
		shas = append(shas, row.Sha)
	}
	return shas
}

func listNamedSHAsForSource(rows []ListLatestNamedSpecsRow, domain, repo string) []string {
	var shas []string
	for _, row := range rows {
		var source struct {
			Domain string `json:"domain"`
			Repo   string `json:"repo"`
		}
		if err := json.Unmarshal(row.Source, &source); err != nil {
			continue
		}
		if source.Domain == domain && source.Repo == repo {
			shas = append(shas, row.Sha)
		}
	}
	return shas
}
