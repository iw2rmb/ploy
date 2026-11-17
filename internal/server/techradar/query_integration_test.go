package techradar

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestQueryObservedSetsLive(t *testing.T) {
	if os.Getenv("PLOY_TRINO_INTEGRATION") != "1" {
		t.Skip("set PLOY_TRINO_INTEGRATION=1 to execute VALUES fixtures through Trino")
	}
	user, password := os.Getenv("PLOY_TRINO_USER"), os.Getenv("PLOY_TRINO_PASSWORD")
	if user == "" || password == "" {
		t.Fatal("PLOY_TRINO_USER and PLOY_TRINO_PASSWORD are required")
	}
	db, err := Open(user, password)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	request := Request{For: []Coordinate{{"g", "a"}, {"g", "b"}}, JDK: 21}
	base := func() [][]any {
		return [][]any{component("c1", "m", "1", "a", "1"), component("c1", "m", "1", "b", "2")}
	}
	java := func() [][]any { return [][]any{metadata("c1", "java_version", "21")} }
	want := [][]string{{"g:a@1", "g:b@2"}}
	type testCase struct {
		name       string
		components [][]any
		metadata   [][]any
		request    Request
		want       [][]string
	}
	tests := []testCase{
		{"complete observed set", base(), java(), request, want},
		{"independent combinations stay separate", [][]any{component("c1", "m", "1", "a", "1"), component("c1", "m", "1", "b", "1"), component("c1", "m", "2", "a", "2"), component("c1", "m", "2", "b", "2")}, java(), request, [][]string{{"g:a@1", "g:b@1"}, {"g:a@2", "g:b@2"}}},
		{"historical observations remain available", append(base(), component("c1", "m", "2", "a", "3")), java(), request, want},
		{"duplicate observations collapse", append(base(), component("c1", "m", "2", "a", "1"), component("c1", "m", "2", "b", "2")), java(), request, want},
		{"multiple versions remain in one set", append(base(), component("c1", "m", "1", "a", "3")), java(), request, [][]string{{"g:a@1", "g:a@3", "g:b@2"}}},
		{"duplicate components collapse", append(base(), component("c1", "m", "1", "a", "1")), java(), request, want},
		{"missing metadata excludes observation", base(), nil, request, [][]string{}},
		{"other commit metadata excludes observation", base(), [][]any{metadata("c2", "java_version", "21")}, request, [][]string{}},
		{"other Java metadata types are ignored", base(), [][]any{metadata("c1", "jvm_version_maven", "21")}, request, [][]string{}},
		{"duplicate agreeing metadata is accepted", base(), append(java(), metadata("c1", "java_version", " 021 ")), request, want},
		{"conflicting metadata excludes observation", base(), append(java(), metadata("c1", "java_version", "17")), request, [][]string{}},
		{"other Java metadata types cannot veto java_version", base(), append(java(), metadata("c1", "jvm_version_maven", "17")), request, want},
	}
	for _, invalid := range []any{nil, "", "21.0", "+21", "0", "-21", "21-ea", "999999999999999999999999999999999999"} {
		row := metadata("c1", "java_version", "")
		row[4] = invalid
		tests = append(tests, testCase{fmt.Sprintf("invalid metadata %v excludes observation", invalid), base(), append(java(), row), request, [][]string{}})
	}
	for index, dimension := range []string{"tenant", "project", "commit", "module", "snapshot"} {
		components := base()
		components[1][index] = "other"
		metadataRows := java()
		if index <= 2 {
			row := metadata("c1", "java_version", "21")
			row[index] = "other"
			metadataRows = append(metadataRows, row)
		}
		tests = append(tests, testCase{dimension + " isolates observations", components, metadataRows, request, [][]string{}})
	}
	for _, index := range []int{0, 1, 2, 3, 4, 6, 7, 8} {
		components := base()
		components[1][index] = nil
		tests = append(tests, testCase{fmt.Sprintf("null component field %d excludes row", index), components, java(), request, [][]string{}})
	}
	for _, index := range []int{2, 6, 7, 8} {
		components := base()
		components[1][index] = " "
		tests = append(tests, testCase{fmt.Sprintf("empty component field %d excludes row", index), components, java(), request, [][]string{}})
	}
	nonJVM := base()
	nonJVM[1][5] = "npm"
	tests = append(tests, testCase{"non-JVM component cannot complete observation", nonJVM, java(), request, [][]string{}})
	for _, version := range []string{"1+build", "1' OR 1=1 --"} {
		constrained := request
		constrained.With = []Constraint{{"g", "anchor", version}}
		for _, matches := range []bool{false, true} {
			components := append(base(), component("c1", "m", "1", "anchor", version+"-different"))
			expected := [][]string{}
			if matches {
				components[2][8] = version
				expected = want
			}
			tests = append(tests, testCase{fmt.Sprintf("opaque constraint %q matches=%t", version, matches), components, java(), constrained, expected})
		}
	}
	constrained := request
	constrained.With = []Constraint{{"g", "a", "1"}, {"g", "b", "2"}}
	tests = append(tests, testCase{"for and with overlap", base(), java(), constrained, want})
	constrained.With = []Constraint{{"g", "anchor", "1"}}
	tests = append(tests, testCase{"constraint from another snapshot cannot match", append(base(), component("c1", "m", "2", "anchor", "1")), java(), constrained, [][]string{}})
	constrained.With = []Constraint{{"g", "a", "1"}, {"g", "a", "3"}}
	tests = append(tests, testCase{"all versions of one constrained coordinate are required", base(), java(), constrained, [][]string{}})
	tests = append(tests, testCase{"multiple constrained versions can coexist", append(base(), component("c1", "m", "1", "a", "3")), java(), constrained, [][]string{{"g:a@1", "g:a@3", "g:b@2"}}})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The production query must return only the fixture's complete observed sets.
			query, args := buildQuery(tt.request)
			query = fixtureQuery(t, query, tt.components, tt.metadata)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			got, err := lookup(ctx, db, query, args)
			if err != nil {
				message := err.Error()
				for _, credential := range []string{user, password} {
					for _, value := range []string{credential, url.QueryEscape(credential), url.PathEscape(credential)} {
						message = strings.ReplaceAll(message, value, "[redacted]")
					}
				}
				t.Fatalf("fixture query failed: %s", message)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("versions = %v, want %v", got, tt.want)
			}
		})
	}
}

func component(commit, module, snapshot, artifact, version string) []any {
	return []any{"tenant", "project", commit, module, snapshot, "jvm", "g", artifact, version}
}

func metadata(commit, kind, value string) []any {
	return []any{"tenant", "project", commit, kind, value}
}

func fixtureQuery(t *testing.T, query string, components, metadata [][]any) string {
	t.Helper()
	// Only test code substitutes data sources; all production matching logic remains unchanged.
	for source, fixture := range map[string]string{
		"dl.prod_sdp_sbomkeeper_aggregator.sbom_components": "fixture_components",
		"dl.prod_sdp_tech_radar.metadata":                   "fixture_metadata",
	} {
		if strings.Count(query, source) != 1 {
			t.Fatalf("production query must reference %s exactly once", source)
		}
		query = strings.ReplaceAll(query, source, fixture)
	}
	return "WITH fixture_components (tenant, project, commit_sha, module_id, created_at, platform, \"group\", name, version) AS (" + fixtureValues(components, 9) + "), fixture_metadata (tenant, repository, commit_sha, type, value) AS (" + fixtureValues(metadata, 5) + "), " + strings.TrimPrefix(query, "WITH ")
}

func fixtureValues(rows [][]any, columns int) string {
	if len(rows) == 0 {
		return "SELECT " + strings.Repeat("CAST(NULL AS VARCHAR), ", columns-1) + "CAST(NULL AS VARCHAR) WHERE false"
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		items := make([]string, len(row))
		for i, item := range row {
			if item == nil {
				items[i] = "CAST(NULL AS VARCHAR)"
			} else {
				items[i] = "'" + strings.ReplaceAll(item.(string), "'", "''") + "'"
			}
		}
		values = append(values, "("+strings.Join(items, ", ")+")")
	}
	return "VALUES " + strings.Join(values, ", ")
}
