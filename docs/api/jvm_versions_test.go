package api_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func TestJVMVersionsSchemaRequiresObservationArrays(t *testing.T) {
	data, err := os.ReadFile("components/schemas/deployments.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var schemas map[string]any
	if err := yaml.Unmarshal(data, &schemas); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("jvm-versions.json", schemas["JVMVersionsResponse"]); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("jvm-versions.json")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		body  string
		valid bool
	}{
		{"empty result is valid", `{"versions":[]}`, true},
		{"empty observation arrays are valid", `{"versions":[{"found":[],"with":[]}]}`, true},
		{"no constraints is valid", `{"versions":[{"found":["g:a@1"],"with":[]}]}`, true},
		{"different constraints preserve observations", `{"versions":[{"found":["g:a@1"],"with":["g:b@2"]},{"found":["g:a@1"],"with":["g:b@3"]}]}`, true},
		{"null version suffix is valid", `{"versions":[{"found":["g:a@"],"with":["g:b@"]}]}`, true},
		{"missing result is invalid", `{}`, false},
		{"null result is invalid", `{"versions":null}`, false},
		{"legacy inner array is invalid", `{"versions":[["g:a@1"]]}`, false},
		{"missing found is invalid", `{"versions":[{"with":[]}]}`, false},
		{"missing with is invalid", `{"versions":[{"found":["g:a@1"]}]}`, false},
		{"null found is invalid", `{"versions":[{"found":null,"with":[]}]}`, false},
		{"null with is invalid", `{"versions":[{"found":["g:a@1"],"with":null}]}`, false},
		{"duplicate found is invalid", `{"versions":[{"found":["g:a@1","g:a@1"],"with":[]}]}`, false},
		{"duplicate with is invalid", `{"versions":[{"found":["g:a@1"],"with":["g:b@2","g:b@2"]}]}`, false},
		{"duplicate observation is invalid", `{"versions":[{"found":["g:a@1"],"with":[]},{"found":["g:a@1"],"with":[]}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var value any
			if err := json.Unmarshal([]byte(test.body), &value); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(value); (err == nil) != test.valid {
				t.Fatalf("Validate = %v; want valid=%v", err, test.valid)
			}
		})
	}

	data, err = os.ReadFile("paths/deployments_jvm_versions.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var path struct {
		Get struct {
			Responses map[string]struct {
				Content map[string]struct {
					Examples map[string]struct {
						Value any `yaml:"value"`
					} `yaml:"examples"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"get"`
	}
	if err := yaml.Unmarshal(data, &path); err != nil {
		t.Fatal(err)
	}
	examples := path.Get.Responses["200"].Content["application/json"].Examples
	if len(examples) == 0 {
		t.Fatal("endpoint has no response examples")
	}
	for name, example := range examples {
		t.Run("example_"+name, func(t *testing.T) {
			if err := schema.Validate(example.Value); err != nil {
				t.Fatalf("response example violates schema: %v", err)
			}
		})
	}
}
