package deployments

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

type fixtureTimestamp string

const (
	fixtureNow                          = "CAST(current_timestamp(6) AT TIME ZONE 'UTC' AS timestamp(6))"
	recentSnapshot     fixtureTimestamp = "date_add('day', -2, " + fixtureNow + ")"
	laterSnapshot      fixtureTimestamp = "date_add('day', -1, " + fixtureNow + ")"
	historicalSnapshot fixtureTimestamp = "date_add('month', -8, " + fixtureNow + ")"
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
	request := Request{For: []Coordinate{{"g", "a"}, {"g", "b"}}, JVM: 21}
	base := func() [][]any {
		return [][]any{component("c1", "m", recentSnapshot, "a", "1"), component("c1", "m", recentSnapshot, "b", "2")}
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
		{"independent combinations stay separate", [][]any{component("c1", "m", recentSnapshot, "a", "1"), component("c1", "m", recentSnapshot, "b", "1"), component("c1", "m", laterSnapshot, "a", "2"), component("c1", "m", laterSnapshot, "b", "2")}, java(), request, [][]string{{"g:a@1", "g:b@1"}, {"g:a@2", "g:b@2"}}},
		{"historical observations within nine months remain available", [][]any{component("c1", "m", historicalSnapshot, "a", "1"), component("c1", "m", historicalSnapshot, "b", "2"), component("c1", "m", laterSnapshot, "a", "3")}, java(), request, want},
		{"duplicate observations collapse", append(base(), component("c1", "m", laterSnapshot, "a", "1"), component("c1", "m", laterSnapshot, "b", "2")), java(), request, want},
		{"multiple versions remain in one set", append(base(), component("c1", "m", recentSnapshot, "a", "3")), java(), request, [][]string{{"g:a@1", "g:a@3", "g:b@2"}}},
		{"duplicate components collapse", append(base(), component("c1", "m", recentSnapshot, "a", "1")), java(), request, want},
		{"missing metadata excludes observation", base(), nil, request, [][]string{}},
		{"other commit metadata excludes observation", base(), [][]any{metadata("c2", "java_version", "21")}, request, [][]string{}},
		{"unsupported metadata type excludes observation", base(), [][]any{metadata("c1", "build_tool", "21")}, request, [][]string{}},
		{"duplicate matching metadata returns one set", base(), append(java(), metadata("c1", "java_version", "21"), metadata("c1", "jvm_version_maven", "21")), request, want},
		{"conflicting metadata cannot veto matching value", base(), append(java(), metadata("c1", "java_version", "17")), request, want},
		{"conflicting metadata type cannot veto matching value", base(), append(java(), metadata("c1", "jvm_version_maven", "17")), request, want},
		{"metadata values with whitespace do not match", base(), [][]any{metadata("c1", "java_version", " 21 ")}, request, [][]string{}},
		{"metadata values with leading zeros do not match", base(), [][]any{metadata("c1", "java_version", "021")}, request, [][]string{}},
	}
	for _, kind := range []string{"jvm_version_yml", "jvm_version_dockerfile", "jvm_version_gradle", "jvm_version_maven", "java_version"} {
		tests = append(tests, testCase{kind + " exact value includes observation", base(), [][]any{metadata("c1", kind, "21")}, request, want})
	}
	for _, commit := range []struct {
		name  string
		value any
		want  [][]string
	}{
		{"matching empty commit values include observation", "", want},
		{"commit matching preserves literal whitespace", " ", want},
		{"null commit values do not join", nil, [][]string{}},
	} {
		components := base()
		for _, row := range components {
			row[2] = commit.value
		}
		metadataRow := metadata("", "java_version", "21")
		metadataRow[2] = commit.value
		tests = append(tests, testCase{commit.name, components, [][]any{metadataRow}, request, commit.want})
	}
	cutoff := "date_add('month', -9, " + fixtureNow + ")"
	for _, boundary := range []struct {
		name     string
		snapshot fixtureTimestamp
		want     [][]string
	}{
		{"observation at nine-month cutoff is included", fixtureTimestamp(cutoff), want},
		{"observation inside nine-month cutoff is included", fixtureTimestamp("date_add('millisecond', 1, " + cutoff + ")"), want},
		{"observation outside nine-month cutoff is excluded", fixtureTimestamp("date_add('millisecond', -1, " + cutoff + ")"), [][]string{}},
	} {
		tests = append(tests, testCase{boundary.name, [][]any{component("c1", "m", boundary.snapshot, "a", "1"), component("c1", "m", boundary.snapshot, "b", "2")}, java(), request, boundary.want})
		metadataRow := metadata("c1", "java_version", "21")
		metadataRow[5] = boundary.snapshot
		tests = append(tests, testCase{strings.Replace(boundary.name, "observation", "metadata", 1), base(), [][]any{metadataRow}, request, boundary.want})
	}
	expiredMatchingMetadata := metadata("c1", "java_version", "21")
	expiredMatchingMetadata[5] = fixtureTimestamp("date_add('millisecond', -1, " + cutoff + ")")
	tests = append(tests, testCase{"expired matching metadata cannot qualify recent nonmatching metadata", base(), [][]any{expiredMatchingMetadata, metadata("c1", "java_version", "17")}, request, [][]string{}})
	for _, invalid := range []any{nil, "", " 21 ", "021", "21.0", "+21", "0", "-21", "21-ea", "999999999999999999999999999999999999"} {
		row := metadata("c1", "java_version", "")
		row[4] = invalid
		tests = append(tests, testCase{fmt.Sprintf("nonmatching metadata %v cannot veto matching value", invalid), base(), append(java(), row), request, want})
	}
	for index, dimension := range []string{"tenant", "project", "commit", "module", "snapshot"} {
		components := base()
		if index == 4 {
			components[1][index] = laterSnapshot
		} else {
			components[1][index] = "other"
		}
		metadataRows := java()
		if index == 2 {
			row := metadata("c1", "java_version", "21")
			row[index] = "other"
			metadataRows = append(metadataRows, row)
		}
		if index >= 2 {
			tests = append(tests, testCase{dimension + " isolates observations", components, metadataRows, request, [][]string{}})
		} else {
			tests = append(tests, testCase{dimension + " differences merge within one commit and snapshot", components, metadataRows, request, want})
		}
	}
	otherMetadata := metadata("c1", "java_version", "21")
	otherMetadata[0], otherMetadata[1] = "other-tenant", "other-repository"
	tests = append(tests, testCase{"metadata matches commit across tenant and repository", base(), [][]any{otherMetadata}, request, want})
	for _, version := range []struct {
		value any
		text  string
	}{{nil, ""}, {"", ""}, {" ", " "}} {
		components := base()
		components[1][8] = version.value
		tests = append(tests, testCase{fmt.Sprintf("selected version %v is retained", version.value), components, java(), request, [][]string{{"g:a@1", "g:b@" + version.text}}})
	}
	// U+001C distinguishes literal coordinate matching from Trino's whitespace trimming.
	for _, index := range []int{6, 7} {
		row := component("c1", "m", recentSnapshot, "a", "1")
		row[index] = "\x1c"
		coordinate := Coordinate{Group: row[6].(string), Artifact: row[7].(string)}
		tests = append(tests, testCase{fmt.Sprintf("selected coordinate field %d is preserved", index), [][]any{row}, java(), Request{For: []Coordinate{coordinate}, JVM: 21}, [][]string{{coordinate.Group + ":" + coordinate.Artifact + "@1"}}})
	}
	nonJVM := base()
	nonJVM[1][5] = "npm"
	tests = append(tests, testCase{"non-JVM component cannot complete observation", nonJVM, java(), request, [][]string{}})
	unrelated := component("c1", "m", recentSnapshot, "a", "99")
	unrelated[6] = "other"
	tests = append(tests, testCase{"unrelated coordinates do not change observed set", append(base(), unrelated, component("c1", "m", recentSnapshot, "unrequested", "99")), java(), request, want})
	pairs := base()
	pairs[1][6] = "other"
	pairRequest := Request{For: []Coordinate{{"g", "a"}, {"other", "b"}}, JVM: 21}
	tests = append(tests, testCase{"coordinate pairs remain exact across groups", append(pairs, unrelated, component("c1", "m", recentSnapshot, "b", "99")), java(), pairRequest, [][]string{{"g:a@1", "other:b@2"}}})
	for _, version := range []string{"1+build", "1' OR 1=1 --"} {
		constrained := request
		constrained.With = []Constraint{{"g", "anchor", version}}
		for _, matches := range []bool{false, true} {
			components := append(base(), component("c1", "m", recentSnapshot, "anchor", version+"-different"))
			expected := [][]string{}
			if matches {
				components[2][8] = version
				expected = want
			}
			tests = append(tests, testCase{fmt.Sprintf("opaque constraint %q matches=%t", version, matches), components, java(), constrained, expected})
		}
	}
	constrained := request
	constrained.With = []Constraint{{"g", "anchor", "1"}}
	tests = append(tests, testCase{"constraint from another snapshot cannot match", append(base(), component("c1", "m", laterSnapshot, "anchor", "1")), java(), constrained, [][]string{}})
	tests = append(tests, testCase{"constraint from another module cannot match", append(base(), component("c1", "other", recentSnapshot, "anchor", "1")), java(), constrained, [][]string{}})
	tests = append(tests, testCase{"qualifying modules return separate version sets", append(base(), component("c1", "m", recentSnapshot, "anchor", "1"), component("c1", "other", recentSnapshot, "a", "3"), component("c1", "other", recentSnapshot, "b", "4"), component("c1", "other", recentSnapshot, "anchor", "1")), java(), constrained, [][]string{{"g:a@1", "g:b@2"}, {"g:a@3", "g:b@4"}}})
	tests = append(tests, testCase{"selected coordinates retain every version with a distinct constraint", append(base(), component("c1", "m", recentSnapshot, "a", "3"), component("c1", "m", recentSnapshot, "b", "4"), component("c1", "m", recentSnapshot, "anchor", "1")), java(), constrained, [][]string{{"g:a@1", "g:a@3", "g:b@2", "g:b@4"}}})
	constrained.With = []Constraint{{"g", "anchor", "1"}, {"g", "other-anchor", "2"}}
	tests = append(tests, testCase{"every distinct constraint is required", append(base(), component("c1", "m", recentSnapshot, "anchor", "1")), java(), constrained, [][]string{}})
	tests = append(tests, testCase{"constraint versions cannot cross coordinates", append(base(), component("c1", "m", recentSnapshot, "anchor", "2"), component("c1", "m", recentSnapshot, "other-anchor", "1")), java(), constrained, [][]string{}})
	tests = append(tests, testCase{"distinct constraints complete the observation without entering projection", append(base(), component("c1", "m", recentSnapshot, "anchor", "1"), component("c1", "m", recentSnapshot, "other-anchor", "2")), java(), constrained, want})
	otherGroup := component("c1", "m", recentSnapshot, "a", "3")
	otherGroup[6] = "other"
	constrained.With = []Constraint{{"other", "a", "3"}}
	tests = append(tests, testCase{"same artifact in another group is a distinct constraint", append(base(), otherGroup), java(), constrained, want})
	tests = append(tests, testCase{"selected artifact in another group cannot satisfy constraint", append(base(), component("c1", "m", recentSnapshot, "a", "3")), java(), constrained, [][]string{}})
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

func component(commit, module string, snapshot fixtureTimestamp, artifact, version string) []any {
	return []any{"tenant", "project", commit, module, snapshot, "jvm", "g", artifact, version}
}

func metadata(commit, kind, value string) []any {
	return []any{"tenant", "project", commit, kind, value, recentSnapshot}
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
	return "WITH fixture_components (tenant, project, commit_sha, module_id, created_at, platform, \"group\", name, version) AS (" + fixtureValues(components, 9) + "), fixture_metadata (tenant, repository, commit_sha, type, value, created_at) AS (" + fixtureValues(metadata, 6) + "), " + strings.TrimPrefix(query, "WITH ")
}

func fixtureValues(rows [][]any, columns int) string {
	if len(rows) == 0 {
		return "SELECT " + strings.Repeat("NULL, ", columns-1) + "NULL WHERE false"
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		items := make([]string, len(row))
		for i, item := range row {
			switch value := item.(type) {
			case nil:
				items[i] = "NULL"
			case fixtureTimestamp:
				items[i] = string(value)
			default:
				items[i] = "'" + strings.ReplaceAll(item.(string), "'", "''") + "'"
			}
		}
		values = append(values, "("+strings.Join(items, ", ")+")")
	}
	return "VALUES " + strings.Join(values, ", ")
}
