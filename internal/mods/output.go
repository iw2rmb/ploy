package mods

import (
	"fmt"
)

func printPlanSummary(b []byte) {
	var parsed struct {
		PlanID  string           `json:"plan_id"`
		Options []map[string]any `json:"options"`
	}
	if err := jsonUnmarshal(b, &parsed); err == nil && parsed.PlanID != "" && len(parsed.Options) > 0 {
		fmt.Printf("Planner produced plan (id=%s) with %d option(s):\n", parsed.PlanID, len(parsed.Options))
		for i, o := range parsed.Options {
			id, _ := o["id"].(string)
			typ, _ := o["type"].(string)
			fmt.Printf("  %d) %s (%s)\n", i+1, id, typ)
		}
	} else {
		fmt.Printf("Planner finished but plan.json validation failed or missing keys: %v\n", err)
	}
}

func printNextSummary(b []byte) {
	var parsed struct {
		Action string `json:"action"`
		Notes  string `json:"notes"`
	}
	if err := jsonUnmarshal(b, &parsed); err == nil && parsed.Action != "" {
		fmt.Printf("Reducer next action: %s", parsed.Action)
		if parsed.Notes != "" {
			fmt.Printf(" (%s)", parsed.Notes)
		}
		fmt.Println()
	} else {
		fmt.Printf("Reducer output invalid or missing keys: %v\n", err)
	}
}
