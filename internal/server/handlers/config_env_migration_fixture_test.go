package handlers

import (
	"os"
	"path/filepath"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"gopkg.in/yaml.v3"
)

// migrationFixture defines the YAML schema for migration dry-run fixture files.
type migrationFixture struct {
	Description  string                                 `yaml:"description"`
	Env          map[string][]migrationFixtureEnvEntry  `yaml:"env"`
	ExistingCA   map[string][]string                    `yaml:"existing_ca"`
	ExistingHome map[string][]migrationFixtureHomeEntry `yaml:"existing_home"`
	Expect       migrationFixtureExpect                 `yaml:"expect"`
}

type migrationFixtureEnvEntry struct {
	Value  string `yaml:"value"`
	Target string `yaml:"target"`
	Secret bool   `yaml:"secret"`
}

type migrationFixtureHomeEntry struct {
	Entry string `yaml:"entry"`
	Dst   string `yaml:"dst"`
}

type migrationFixtureExpect struct {
	Rewritten int                              `yaml:"rewritten"`
	Rejected  int                              `yaml:"rejected"`
	Skipped   int                              `yaml:"skipped"`
	Entries   []migrationFixtureExpectedEntry  `yaml:"entries"`
}

type migrationFixtureExpectedEntry struct {
	EnvKey      string   `yaml:"env_key"`
	Target      string   `yaml:"target"`
	Action      string   `yaml:"action"`
	TargetField string   `yaml:"target_field"`
	Destination string   `yaml:"destination,omitempty"`
	Sections    []string `yaml:"sections,omitempty"`
}

// TestMigrationDryRunFixtures loads each YAML fixture from testdata/migration_fixtures
// and asserts that ScanSpecialEnvKeys produces the expected migration report.
func TestMigrationDryRunFixtures(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "migration_fixtures")
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no fixtures found in testdata/migration_fixtures")
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			var fix migrationFixture
			if err := yaml.Unmarshal(data, &fix); err != nil {
				t.Fatalf("parse fixture: %v", err)
			}

			// Build inputs from fixture.
			globalEnv := make(map[string][]GlobalEnvVar)
			for key, entries := range fix.Env {
				for _, e := range entries {
					target, err := domaintypes.ParseGlobalEnvTarget(e.Target)
					if err != nil {
						t.Fatalf("invalid target %q for key %q: %v", e.Target, key, err)
					}
					globalEnv[key] = append(globalEnv[key], GlobalEnvVar{
						Value:  e.Value,
						Target: target,
						Secret: e.Secret,
					})
				}
			}

			existingHome := make(map[string][]ConfigHomeEntry)
			for section, entries := range fix.ExistingHome {
				for _, e := range entries {
					existingHome[section] = append(existingHome[section], ConfigHomeEntry{
						Entry:   e.Entry,
						Dst:     e.Dst,
						Section: section,
					})
				}
			}

			// Run scan.
			report := ScanSpecialEnvKeys(globalEnv, fix.ExistingCA, existingHome)

			// Assert counts.
			if report.Rewritten != fix.Expect.Rewritten {
				t.Errorf("Rewritten = %d, want %d", report.Rewritten, fix.Expect.Rewritten)
			}
			if report.Rejected != fix.Expect.Rejected {
				t.Errorf("Rejected = %d, want %d", report.Rejected, fix.Expect.Rejected)
			}
			if report.Skipped != fix.Expect.Skipped {
				t.Errorf("Skipped = %d, want %d", report.Skipped, fix.Expect.Skipped)
			}
			if len(report.Entries) != len(fix.Expect.Entries) {
				t.Fatalf("Entries count = %d, want %d", len(report.Entries), len(fix.Expect.Entries))
			}

			// Assert individual entries (fixture entries are in sorted order).
			for i, want := range fix.Expect.Entries {
				got := report.Entries[i]

				if got.EnvKey != want.EnvKey {
					t.Errorf("entry[%d].EnvKey = %q, want %q", i, got.EnvKey, want.EnvKey)
				}
				if got.Target != want.Target {
					t.Errorf("entry[%d].Target = %q, want %q", i, got.Target, want.Target)
				}
				if string(got.Action) != want.Action {
					t.Errorf("entry[%d].Action = %q, want %q", i, got.Action, want.Action)
				}
				if got.TargetField != want.TargetField {
					t.Errorf("entry[%d].TargetField = %q, want %q", i, got.TargetField, want.TargetField)
				}
				if want.Destination != "" && got.Destination != want.Destination {
					t.Errorf("entry[%d].Destination = %q, want %q", i, got.Destination, want.Destination)
				}
				if len(want.Sections) > 0 {
					if len(got.Sections) != len(want.Sections) {
						t.Errorf("entry[%d].Sections = %v, want %v", i, got.Sections, want.Sections)
					} else {
						for j := range want.Sections {
							if got.Sections[j] != want.Sections[j] {
								t.Errorf("entry[%d].Sections[%d] = %q, want %q", i, j, got.Sections[j], want.Sections[j])
							}
						}
					}
				}
			}
		})
	}
}
