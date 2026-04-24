package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	mavenLegacyBuildCommand = "mvn --ff -B -q -e -DskipTests=true -Dstyle.color=never -f /workspace/pom.xml clean install"
	mavenLegacyUnitCommand  = "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test"
	mavenLegacyTestsCommand = "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install"

	mavenWrapperCompile = "./mvnw -B -e clean compile"
)

type mavenGateProfileReconcileStats struct {
	Scanned   int
	Rewritten int
	Unchanged int
}

type mavenGateProfileBlobRef struct {
	ID        int64
	ObjectKey string
}

type mavenGateProfileReconcileStore interface {
	ListMavenGateProfiles(ctx context.Context) ([]mavenGateProfileBlobRef, error)
}

type sqlMavenGateProfileReconcileStore struct {
	pool *pgxpool.Pool
}

func newSQLMavenGateProfileReconcileStore(pool *pgxpool.Pool) mavenGateProfileReconcileStore {
	return &sqlMavenGateProfileReconcileStore{pool: pool}
}

func (s *sqlMavenGateProfileReconcileStore) ListMavenGateProfiles(ctx context.Context) ([]mavenGateProfileBlobRef, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("maven gate profile reconcile: store pool is required")
	}
	rows, err := s.pool.Query(ctx, `
SELECT gp.id,
       gp.url
FROM gate_profiles gp
JOIN stacks s ON s.id = gp.stack_id
WHERE s.lang = 'java'
  AND COALESCE(s.tool, '') = 'maven'
ORDER BY gp.id ASC
`)
	if err != nil {
		return nil, fmt.Errorf("list maven gate profiles: %w", err)
	}
	defer rows.Close()

	out := make([]mavenGateProfileBlobRef, 0)
	for rows.Next() {
		var row mavenGateProfileBlobRef
		if err := rows.Scan(&row.ID, &row.ObjectKey); err != nil {
			return nil, fmt.Errorf("scan maven gate profile row: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate maven gate profile rows: %w", err)
	}
	return out, nil
}

func reconcileMavenGateProfiles(
	ctx context.Context,
	st mavenGateProfileReconcileStore,
	bs blobstore.Store,
) (mavenGateProfileReconcileStats, error) {
	if st == nil {
		return mavenGateProfileReconcileStats{}, fmt.Errorf("maven gate profile reconcile: store is required")
	}
	if bs == nil {
		return mavenGateProfileReconcileStats{}, fmt.Errorf("maven gate profile reconcile: blobstore is required")
	}

	rows, err := st.ListMavenGateProfiles(ctx)
	if err != nil {
		return mavenGateProfileReconcileStats{}, err
	}
	stats := mavenGateProfileReconcileStats{
		Scanned: len(rows),
	}
	for _, row := range rows {
		key := strings.TrimSpace(row.ObjectKey)
		if key == "" {
			return stats, fmt.Errorf("maven gate profile reconcile: empty object key for profile_id=%d", row.ID)
		}
		raw, err := blobstore.ReadAll(ctx, bs, key)
		if err != nil {
			return stats, fmt.Errorf("maven gate profile reconcile: read profile_id=%d key=%q: %w", row.ID, key, err)
		}
		rewritten, changed, err := rewriteMavenGateProfileCommands(raw)
		if err != nil {
			return stats, fmt.Errorf("maven gate profile reconcile: profile_id=%d key=%q: %w", row.ID, key, err)
		}
		if !changed {
			stats.Unchanged++
			continue
		}
		if _, err := bs.Put(ctx, key, contentTypeJSON, rewritten); err != nil {
			return stats, fmt.Errorf("maven gate profile reconcile: write profile_id=%d key=%q: %w", row.ID, key, err)
		}
		stats.Rewritten++
	}
	return stats, nil
}

func rewriteMavenGateProfileCommands(raw []byte) ([]byte, bool, error) {
	profile, err := contracts.ParseGateProfileJSON(raw)
	if err != nil {
		return nil, false, fmt.Errorf("parse gate profile: %w", err)
	}

	rewritten := false
	if rewriteMavenTargetCommand(profile.Targets.Build, mavenLegacyBuildCommand) {
		rewritten = true
	}
	if rewriteMavenTargetCommand(profile.Targets.Unit, mavenLegacyUnitCommand) {
		rewritten = true
	}
	if rewriteMavenTargetCommand(profile.Targets.AllTests, mavenLegacyTestsCommand) {
		rewritten = true
	}
	if !rewritten {
		return raw, false, nil
	}

	updated, err := json.Marshal(profile)
	if err != nil {
		return nil, false, fmt.Errorf("marshal rewritten gate profile: %w", err)
	}
	if _, err := contracts.ParseGateProfileJSON(updated); err != nil {
		return nil, false, fmt.Errorf("validate rewritten gate profile: %w", err)
	}
	return updated, true, nil
}

func rewriteMavenTargetCommand(target *contracts.GateProfileTarget, fallback string) bool {
	if target == nil {
		return false
	}
	if target.Command != fallback {
		return false
	}
	target.Command = mavenWrapperConditionalCommand(fallback)
	return true
}

func mavenWrapperConditionalCommand(fallback string) string {
	return fmt.Sprintf("if [ -f /workspace/mvnw ]; then %s; else %s; fi", mavenWrapperCompile, fallback)
}
