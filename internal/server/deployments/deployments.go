package deployments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/trinodb/trino-go-client/trino"

	"github.com/iw2rmb/ploy/internal/domain/jvmversions"
)

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
func (s *Service) Lookup(ctx context.Context, request jvmversions.Request) ([]jvmversions.Observation, error) {
	query, args := buildQuery(request)
	return lookup(ctx, s.db, query, args)
}

func lookup(ctx context.Context, db *sql.DB, query string, args []any) ([]jvmversions.Observation, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	defer rows.Close()
	versions := make([]jvmversions.Observation, 0)
	for rows.Next() {
		var found, with string
		if err := rows.Scan(&found, &with); err != nil {
			return nil, err
		}
		var observed jvmversions.Observation
		if err := json.Unmarshal([]byte(found), &observed.Found); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(with), &observed.With); err != nil {
			return nil, err
		}
		versions = append(versions, observed)
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
	return versions, nil
}

func buildQuery(request jvmversions.Request) (string, []any) {
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
	appendPredicate := func(groupColumn, versionColumn string, coordinate jvmversions.Coordinate, selector jvmversions.Selector) {
		query.WriteString("(" + groupColumn + " = CAST(? AS VARCHAR) AND name = CAST(? AS VARCHAR)")
		args = append(args, coordinate.Group, coordinate.Artifact)
		switch selector.Kind {
		case jvmversions.Exact:
			query.WriteString(" AND " + versionColumn + " = CAST(? AS VARCHAR)")
			args = append(args, selector.Value)
		case jvmversions.Prefix:
			query.WriteString(" AND starts_with(" + versionColumn + ", CAST(? AS VARCHAR))")
			args = append(args, selector.Value)
		}
		query.WriteString(")")
	}
	for i, coordinate := range request.For {
		if i > 0 {
			query.WriteString(" OR ")
		}
		appendPredicate(`c."group"`, "c.version", coordinate, jvmversions.Selector{Kind: jvmversions.Any})
	}
	for _, constraint := range request.With {
		query.WriteString(" OR ")
		appendPredicate(`c."group"`, "c.version", constraint.Coordinate, constraint.Selector)
	}
	// Module and snapshot boundaries prevent unrelated components from completing an observation.
	query.WriteString(`)
), observations AS (
    SELECT commit_sha, created_at, module_id,
        coalesce(array_agg(group_id || ':' || name || '@' || coalesce(version, ''))
            FILTER (WHERE `)
	for i, coordinate := range request.For {
		if i > 0 {
			query.WriteString(" OR ")
		}
		appendPredicate("group_id", "version", coordinate, jvmversions.Selector{Kind: jvmversions.Any})
	}
	query.WriteString(`), CAST(ARRAY[] AS ARRAY(VARCHAR))) AS found,
        coalesce(array_agg(group_id || ':' || name || '@' || coalesce(version, ''))
            FILTER (WHERE `)
	if len(request.With) == 0 {
		query.WriteString("false")
	}
	for i, constraint := range request.With {
		if i > 0 {
			query.WriteString(" OR ")
		}
		// Source filtering already applied selectors; list membership determines projection.
		appendPredicate("group_id", "version", constraint.Coordinate, jvmversions.Selector{Kind: jvmversions.Any})
	}
	query.WriteString(`), CAST(ARRAY[] AS ARRAY(VARCHAR))) AS matched_with
    FROM components
    GROUP BY commit_sha, created_at, module_id
    HAVING `)
	for i, coordinate := range request.For {
		if i > 0 {
			query.WriteString("\n       AND ")
		}
		query.WriteString("count_if(")
		appendPredicate("group_id", "version", coordinate, jvmversions.Selector{Kind: jvmversions.Any})
		query.WriteString(") > 0")
	}
	for _, constraint := range request.With {
		query.WriteString("\n       AND count_if(")
		appendPredicate("group_id", "version", constraint.Coordinate, constraint.Selector)
		query.WriteString(") > 0")
	}
	query.WriteString(`
)
SELECT json_format(CAST(found AS JSON)), json_format(CAST(matched_with AS JSON))
FROM observations
`)
	return query.String(), args
}
