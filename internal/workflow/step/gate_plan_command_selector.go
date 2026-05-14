package step

func resolveGateCommand(
	_ string,
	_ string,
	_ string,
	_ string,
) ([]string, map[string]string, error) {
	// Gate images own their runtime command now. Ploy only selects image/env.
	return nil, nil, nil
}
