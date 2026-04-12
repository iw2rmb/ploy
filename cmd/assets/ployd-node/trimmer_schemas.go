package ploydnodeassets

import (
	"embed"
	"fmt"
	"io/fs"
)

var (
	//go:embed *.trimmer.schema.json
	trimmerSchemasFS embed.FS
)

func ReadTrimmerSchema(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("trimmer schema name is empty")
	}
	data, err := trimmerSchemasFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func HasTrimmerSchema(name string) bool {
	if name == "" {
		return false
	}
	_, err := trimmerSchemasFS.ReadFile(name)
	return err == nil
}

func IsMissingTrimmerSchema(err error) bool {
	return err != nil && (err == fs.ErrNotExist || isPathErrorNotExist(err))
}

func isPathErrorNotExist(err error) bool {
	if pe, ok := err.(*fs.PathError); ok {
		return pe.Err == fs.ErrNotExist
	}
	return false
}
