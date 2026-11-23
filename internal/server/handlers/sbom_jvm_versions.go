package handlers

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/iw2rmb/ploy/internal/domain/jvmversions"
	"github.com/iw2rmb/ploy/internal/store"
)

type sbomJVMVersionsLookup struct {
	store store.Store
}

func (lookup sbomJVMVersionsLookup) Lookup(ctx context.Context, request jvmversions.Request) ([]jvmversions.Observation, error) {
	type entry struct {
		Lib        string `json:"lib"`
		Kind       string `json:"kind"`
		Value      string `json:"value"`
		Membership string `json:"membership"`
	}
	kinds := [...]string{jvmversions.Any: "any", jvmversions.Exact: "exact", jvmversions.Prefix: "prefix"}
	entries := make([]entry, 0, len(request.For)+len(request.With))
	for _, coord := range request.For {
		entries = append(entries, entry{
			Lib: coord.Group + ":" + coord.Artifact, Kind: kinds[jvmversions.Any], Membership: "for",
		})
	}
	for _, constraint := range request.With {
		coord := constraint.Coordinate
		entries = append(entries, entry{
			Lib: coord.Group + ":" + coord.Artifact, Kind: kinds[constraint.Selector.Kind],
			Value: constraint.Selector.Value, Membership: "with",
		})
	}
	// Every entry must match UTF-8 PostgreSQL text. JSON encoding replaces invalid bytes.
	storable := func(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
	for _, e := range entries {
		if !storable(e.Lib) || !storable(e.Value) {
			return nil, nil
		}
	}
	requested, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	rows, err := lookup.store.ListSBOMJVMVersions(ctx, store.ListSBOMJVMVersionsParams{
		Jvm: strconv.FormatInt(request.JVM, 10), Requested: requested,
	})
	if err != nil {
		return nil, err
	}
	observations := make([]jvmversions.Observation, len(rows))
	for i, row := range rows {
		observations[i] = jvmversions.Observation{Found: row.Found, With: row.With}
	}
	return observations, nil
}
