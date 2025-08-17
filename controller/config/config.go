package config

import (
	"os"
	"gopkg.in/yaml.v3"
	"github.com/ploy/ploy/internal/storage"
)

type Root struct {
	Storage storage.Config `yaml:"storage"`
}

func Load(path string) (Root, error) {
	b, err := os.ReadFile(path); if err != nil { return Root{}, err }
	var r Root
	if err := yaml.Unmarshal(b, &r); err != nil { return Root{}, err }
	return r, nil
}
