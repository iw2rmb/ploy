package deployments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
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
	JVM  int64
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

// Lookup requires validated inputs with unique, disjoint For and With coordinates.
// The caller owns the lookup deadline.
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
	args := []any{strconv.FormatInt(request.JVM, 10)}
	// UTC keeps the observation window independent of the driver's local session time zone.
	query.WriteString(`WITH jvm AS (
    SELECT DISTINCT commit_sha
    FROM dl.prod_sdp_tech_radar.metadata
    WHERE type IN ('jvm_version_yml', 'jvm_version_dockerfile', 'jvm_version_gradle', 'jvm_version_maven', 'java_version')
      AND value = ?
      AND created_at >= date_add('month', -9, CAST(current_timestamp(6) AT TIME ZONE 'UTC' AS timestamp(6)))
), components AS (
    SELECT c.commit_sha, c.created_at, c.module_id, c."group" AS group_id, c.name, c.version
    FROM dl.prod_sdp_sbomkeeper_aggregator.sbom_components c
    JOIN jvm j ON j.commit_sha = c.commit_sha
    WHERE c.platform = 'jvm'
      AND c.created_at >= date_add('month', -9, CAST(current_timestamp(6) AT TIME ZONE 'UTC' AS timestamp(6)))
      AND (`)
	appendPair := func(groupColumn string, coordinate Coordinate) {
		query.WriteString("(" + groupColumn + " = CAST(? AS VARCHAR) AND name = CAST(? AS VARCHAR))")
		args = append(args, coordinate.Group, coordinate.Artifact)
	}
	for i, coordinate := range request.For {
		if i > 0 {
			query.WriteString(" OR ")
		}
		appendPair(`c."group"`, coordinate)
	}
	for _, constraint := range request.With {
		query.WriteString(" OR (")
		appendPair(`c."group"`, Coordinate{constraint.Group, constraint.Artifact})
		query.WriteString(" AND c.version = CAST(? AS VARCHAR))")
		args = append(args, constraint.Version)
	}
	// Module and snapshot boundaries prevent unrelated components from completing an observation.
	query.WriteString(`)
), observations AS (
    SELECT commit_sha, created_at, module_id,
        array_sort(array_distinct(array_agg(group_id || ':' || name || '@' || coalesce(version, ''))
            FILTER (WHERE `)
	for i, coordinate := range request.For {
		if i > 0 {
			query.WriteString(" OR ")
		}
		appendPair("group_id", coordinate)
	}
	query.WriteString(`))) AS versions
    FROM components
    GROUP BY commit_sha, created_at, module_id
    HAVING `)
	for i, coordinate := range request.For {
		if i > 0 {
			query.WriteString("\n       AND ")
		}
		query.WriteString("count_if(")
		appendPair("group_id", coordinate)
		query.WriteString(") > 0")
	}
	for _, constraint := range request.With {
		query.WriteString("\n       AND count_if(")
		appendPair("group_id", Coordinate{constraint.Group, constraint.Artifact})
		query.WriteString(" AND version = CAST(? AS VARCHAR)) > 0")
		args = append(args, constraint.Version)
	}
	query.WriteString(`
)
SELECT json_format(CAST(versions AS JSON))
FROM observations
`)
	return query.String(), args
}
