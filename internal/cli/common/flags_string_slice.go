package common

import "strings"

// StringSlice is a simple flag.Value for collecting repeated values.
type StringSlice []string

func (s *StringSlice) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *StringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}
