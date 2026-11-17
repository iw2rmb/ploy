package techradar

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/trinodb/trino-go-client/trino"
)

type Coordinate struct {
	Group    string
	Artifact string
}

type Constraint struct {
	Group    string
	Artifact string
	Version  string
}

type Request struct {
	For  []Coordinate
	With []Constraint
	JDK  int64
}

type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service { return &Service{db: db} }

func Open(user, password string) (*sql.DB, error) {
	server := url.URL{Scheme: "https", Host: "trino.dlh.t-infra.ru", User: url.UserPassword(user, password)}
	configuration := trino.Config{ServerURI: server.String(), Source: "ploy"}
	dsn, err := configuration.FormatDSN()
	if err != nil {
		return nil, errors.New("invalid Trino configuration")
	}
	db, err := sql.Open("trino", dsn)
	if err != nil {
		return nil, errors.New("could not initialize Trino")
	}
	return db, nil
}

// Lookup requires validated, deduplicated inputs. The caller owns the lookup deadline.
func (s *Service) Lookup(ctx context.Context, request Request) ([][]string, error) {
	query, args := buildQuery(request)
	return lookup(ctx, s.db, query, args)
}

func lookup(ctx context.Context, db *sql.DB, query string, args []any) ([][]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	defer rows.Close()
	versions := make([][]string, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var observed []string
		if err := json.Unmarshal([]byte(encoded), &observed); err != nil {
			return nil, err
		}
		slices.Sort(observed)
		observed = slices.Compact(observed)
		key, err := json.Marshal(observed)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[string(key)]; !exists {
			seen[string(key)] = struct{}{}
			versions = append(versions, observed)
		}
	}
	iterationErr := rows.Err()
	closeErr := rows.Close()
	// Cancellation can fail after the lookup deadline; the deadline remains the failure cause.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if iterationErr != nil {
		return nil, iterationErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	slices.SortFunc(versions, slices.Compare[[]string])
	return versions, nil
}

func buildQuery(request Request) (string, []any) {
	var query strings.Builder
	args := make([]any, 0, len(request.For)*2+len(request.With)*3+1)
	query.WriteString("WITH requested_for (group_id, artifact) AS (VALUES ")
	for i, coordinate := range request.For {
		if i > 0 {
			query.WriteString(", ")
		}
		query.WriteString("(CAST(? AS VARCHAR), CAST(? AS VARCHAR))")
		args = append(args, coordinate.Group, coordinate.Artifact)
	}
	query.WriteString("), requested_with (group_id, artifact, version) AS (")
	if len(request.With) == 0 {
		query.WriteString("SELECT CAST(NULL AS VARCHAR), CAST(NULL AS VARCHAR), CAST(NULL AS VARCHAR) WHERE false")
	} else {
		query.WriteString("VALUES ")
		for i, constraint := range request.With {
			if i > 0 {
				query.WriteString(", ")
			}
			query.WriteString("(CAST(? AS VARCHAR), CAST(? AS VARCHAR), CAST(? AS VARCHAR))")
			args = append(args, constraint.Group, constraint.Artifact, constraint.Version)
		}
	}
	query.WriteString(observationQuery)
	args = append(args, request.JDK)
	return query.String(), args
}

// Matching uses the complete observation key so versions from different snapshots cannot combine.
const observationQuery = `), java_metadata AS (
    SELECT tenant, repository, commit_sha
    FROM dl.prod_sdp_tech_radar.metadata
    WHERE type = 'java_version'
      AND commit_sha IS NOT NULL AND trim(commit_sha) <> ''
    GROUP BY tenant, repository, commit_sha
    HAVING count_if(coalesce(
        regexp_like(trim(value), '^[0-9]+$')
        AND try_cast(trim(value) AS BIGINT) = CAST(? AS BIGINT), false)) = count(*)
), components AS (
    SELECT tenant, project, commit_sha, module_id, created_at, "group" AS group_id, name, version
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components
    WHERE platform = 'jvm'
      AND tenant IS NOT NULL AND project IS NOT NULL
      AND module_id IS NOT NULL AND created_at IS NOT NULL
      AND commit_sha IS NOT NULL AND trim(commit_sha) <> ''
      AND "group" IS NOT NULL AND trim("group") <> ''
      AND name IS NOT NULL AND trim(name) <> ''
      AND version IS NOT NULL AND trim(version) <> ''
), observations AS (
    SELECT c.tenant, c.project, c.commit_sha, c.module_id, c.created_at,
        array_sort(array_distinct(array_agg(c.group_id || ':' || c.name || '@' || c.version)
            FILTER (WHERE f.group_id IS NOT NULL))) AS versions
    FROM components c
    JOIN java_metadata j ON j.tenant = c.tenant AND j.repository = c.project AND j.commit_sha = c.commit_sha
    LEFT JOIN requested_for f ON f.group_id = c.group_id AND f.artifact = c.name
    LEFT JOIN requested_with w ON w.group_id = c.group_id AND w.artifact = c.name AND w.version = c.version
    WHERE f.group_id IS NOT NULL OR w.group_id IS NOT NULL
    GROUP BY c.tenant, c.project, c.commit_sha, c.module_id, c.created_at
    HAVING count(DISTINCT f.group_id || ':' || f.artifact) = (SELECT count(*) FROM requested_for)
       AND count(DISTINCT w.group_id || ':' || w.artifact || '@' || w.version) = (SELECT count(*) FROM requested_with)
)
SELECT json_format(CAST(versions AS JSON)) FROM observations
`
