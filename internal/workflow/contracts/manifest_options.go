package contracts

// OptionString retrieves a string option by key. Returns the value and true if
// present and convertible to string, otherwise returns empty string and false.
func (m StepManifest) OptionString(key string) (string, bool) {
	if m.Options == nil {
		return "", false
	}
	v, ok := m.Options[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// OptionBool retrieves a boolean option by key. Returns the value and true if
// present and convertible to bool, otherwise returns false and false.
func (m StepManifest) OptionBool(key string) (bool, bool) {
	if m.Options == nil {
		return false, false
	}
	v, ok := m.Options[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}
