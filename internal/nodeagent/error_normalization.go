package nodeagent

func normalizedExecutionError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
