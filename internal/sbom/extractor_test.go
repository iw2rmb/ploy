package sbom

import (
	"testing"
)

func TestExtractPackagesFromJSON_ParsesAndNormalizesSupportedFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []Package
	}{
		{
			name: "SPDX",
			raw: `{
			  "spdxVersion": "SPDX-2.3",
			  "packages": [
			    {"name":"Org.Example:Lib-A","versionInfo":"1.0.0"},
			    {"name":"org.example:lib-a","versionInfo":"1.0.0"},
			    {"name":"org.example:lib-b","versionInfo":"2.0.0"},
			    {"name":"skip-empty-version","versionInfo":""}
			  ]
			}`,
			want: []Package{{Name: "org.example:lib-a", Version: "1.0.0"}, {Name: "org.example:lib-b", Version: "2.0.0"}},
		},
		{
			name: "CycloneDX",
			raw: `{
			  "bomFormat":"CycloneDX",
			  "components":[
			    {"name":"com.fasterxml.jackson.core:jackson-databind","version":"2.18.2"},
			    {"name":"parent","version":"1.0.0","components":[
			      {"name":"nested-lib","version":"3.4.5"}
			    ]}
			  ]
			}`,
			want: []Package{
				{Name: "com.fasterxml.jackson.core:jackson-databind", Version: "2.18.2"},
				{Name: "nested-lib", Version: "3.4.5"},
				{Name: "parent", Version: "1.0.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractPackagesFromJSON([]byte(tt.raw))
			if err != nil {
				t.Fatalf("ExtractPackagesFromJSON error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("package count = %d, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("packages[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
