package nodeagent

func stageIDFromOptions(opts map[string]any) string {
	if opts == nil {
		return ""
	}
	if s, ok := opts["stage_id"].(string); ok {
		return s
	}
	return ""
}
