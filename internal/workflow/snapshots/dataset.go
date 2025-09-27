package snapshots

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type dataset map[string][]row

type row map[string]string

type orderedField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type orderedRow struct {
	Fields []orderedField `json:"fields"`
}

type orderedTable struct {
	Name string       `json:"name"`
	Rows []orderedRow `json:"rows"`
}

type orderedDataset struct {
	Tables []orderedTable `json:"tables"`
}

// loadFixture loads a JSON fixture into the dataset structure used for transformations.
func loadFixture(path string) (dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", path, err)
	}
	var raw map[string][]map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode fixture %s: %w", path, err)
	}
	result := make(dataset, len(raw))
	for table, rows := range raw {
		converted := make([]row, 0, len(rows))
		for _, r := range rows {
			out := make(row, len(r))
			for key, value := range r {
				out[key] = fmt.Sprint(value)
			}
			converted = append(converted, out)
		}
		result[table] = converted
	}
	return result, nil
}

// applyStrip removes configured columns from the dataset and records them in the diff summary.
func applyStrip(data dataset, rules []StripRule, diff DiffSummary) error {
	for _, rule := range rules {
		rows, ok := data[rule.Table]
		if !ok {
			return fmt.Errorf("%w: table %s missing for strip", ErrInvalidRule, rule.Table)
		}
		touched := make(map[string]struct{})
		for i := range rows {
			for _, column := range rule.Columns {
				if _, exists := rows[i][column]; exists {
					delete(rows[i], column)
					touched[column] = struct{}{}
				}
			}
		}
		if len(touched) == 0 {
			return fmt.Errorf("%w: strip columns not found on table %s", ErrInvalidRule, rule.Table)
		}
		diff.StrippedColumns[rule.Table] = mergeAndSort(diff.StrippedColumns[rule.Table], touched)
	}
	return nil
}

// applyMask applies masking strategies and records affected columns in the diff summary.
func applyMask(data dataset, rules []MaskRule, diff DiffSummary) error {
	for _, rule := range rules {
		rows, ok := data[rule.Table]
		if !ok {
			return fmt.Errorf("%w: table %s missing for mask", ErrInvalidRule, rule.Table)
		}
		found := false
		for idx := range rows {
			value, exists := rows[idx][rule.Column]
			if !exists {
				continue
			}
			masked, err := applyMaskStrategy(rule.Strategy, value, rule.Table, idx)
			if err != nil {
				return err
			}
			rows[idx][rule.Column] = masked
			found = true
		}
		if !found {
			return fmt.Errorf("%w: column %s missing for mask", ErrInvalidRule, rule.Column)
		}
		diff.MaskedColumns[rule.Table] = mergeAndSort(diff.MaskedColumns[rule.Table], map[string]struct{}{rule.Column: struct{}{}})
	}
	return nil
}

// applyMaskStrategy transforms a value according to the requested masking strategy.
func applyMaskStrategy(strategy, value, table string, rowIndex int) (string, error) {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "hash":
		sum := sha256.Sum256([]byte(value))
		return fmt.Sprintf("hash-%x", sum[:8]), nil
	case "redact":
		return "REDACTED", nil
	case "last4":
		return maskLast4(value), nil
	default:
		return "", fmt.Errorf("%w: mask strategy %s for table %s", ErrInvalidRule, strategy, table)
	}
}

// maskLast4 preserves only the last four characters of a value for reporting.
func maskLast4(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "last4-"
	}
	runes := []rune(trimmed)
	if len(runes) > 4 {
		runes = runes[len(runes)-4:]
	}
	return "last4-" + string(runes)
}

// applySynthetic generates synthetic values and records touched columns in the diff summary.
func applySynthetic(data dataset, rules []SyntheticRule, diff DiffSummary) error {
	for _, rule := range rules {
		rows, ok := data[rule.Table]
		if !ok {
			return fmt.Errorf("%w: table %s missing for synthetic", ErrInvalidRule, rule.Table)
		}
		switch strings.ToLower(strings.TrimSpace(rule.Strategy)) {
		case "uuid":
			for idx := range rows {
				rows[idx][rule.Column] = fmt.Sprintf("uuid-%s-%d", rule.Table, idx+1)
			}
		case "static":
			for idx := range rows {
				rows[idx][rule.Column] = "STATIC"
			}
		default:
			return fmt.Errorf("%w: synthetic strategy %s for table %s", ErrInvalidRule, rule.Strategy, rule.Table)
		}
		diff.SyntheticColumns[rule.Table] = mergeAndSort(diff.SyntheticColumns[rule.Table], map[string]struct{}{rule.Column: struct{}{}})
	}
	return nil
}

// mergeAndSort merges column names and returns them sorted for deterministic output.
func mergeAndSort(existing []string, additions map[string]struct{}) []string {
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for value := range additions {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// encodeDataset orders tables and fields so the fixture round-trips deterministically.
func encodeDataset(data dataset) ([]byte, error) {
	tableNames := make([]string, 0, len(data))
	for table := range data {
		tableNames = append(tableNames, table)
	}
	sort.Strings(tableNames)

	ordered := orderedDataset{Tables: make([]orderedTable, 0, len(tableNames))}
	for _, name := range tableNames {
		rows := data[name]
		orderedRows := make([]orderedRow, 0, len(rows))
		for _, r := range rows {
			fields := make([]orderedField, 0, len(r))
			keys := make([]string, 0, len(r))
			for key := range r {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fields = append(fields, orderedField{Name: key, Value: r[key]})
			}
			orderedRows = append(orderedRows, orderedRow{Fields: fields})
		}
		ordered.Tables = append(ordered.Tables, orderedTable{Name: name, Rows: orderedRows})
	}

	dataBytes, err := json.Marshal(ordered)
	if err != nil {
		return nil, fmt.Errorf("encode dataset: %w", err)
	}
	return dataBytes, nil
}
