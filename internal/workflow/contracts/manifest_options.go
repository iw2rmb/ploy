package contracts

// OptionString returns the option value for key when it is of type string.
// It returns the string and true on exact type match; otherwise it returns
// an empty string and false. The lookup is safe on a zero-value manifest
// or when Options is nil.
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

// OptionBool returns the option value for key when it is of type bool.
// It returns the bool and true on exact type match; otherwise it returns
// false and false. The lookup is safe on a zero-value manifest or when
// Options is nil.
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
