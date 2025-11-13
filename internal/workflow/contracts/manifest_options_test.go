package contracts

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestStepManifest_OptionString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		options map[string]any
		key     string
		want    string
		wantOk  bool
	}{
		{
			name:    "nil options",
			options: nil,
			key:     "foo",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "empty options",
			options: map[string]any{},
			key:     "foo",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "key not found",
			options: map[string]any{"bar": "baz"},
			key:     "foo",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "value is string",
			options: map[string]any{"foo": "bar"},
			key:     "foo",
			want:    "bar",
			wantOk:  true,
		},
		{
			name:    "value is empty string",
			options: map[string]any{"foo": ""},
			key:     "foo",
			want:    "",
			wantOk:  true,
		},
		{
			name:    "value is not string (bool)",
			options: map[string]any{"foo": true},
			key:     "foo",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "value is not string (int)",
			options: map[string]any{"foo": 42},
			key:     "foo",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "value is not string (map)",
			options: map[string]any{"foo": map[string]any{"nested": "value"}},
			key:     "foo",
			want:    "",
			wantOk:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := StepManifest{
				ID:      types.StepID("test-step"),
				Name:    "Test Step",
				Image:   "test:latest",
				Inputs:  []StepInput{{Name: "test", MountPath: "/test", Mode: StepInputModeReadOnly, SnapshotCID: "cid"}},
				Options: tt.options,
			}
			got, ok := m.OptionString(tt.key)
			if got != tt.want {
				t.Errorf("OptionString() value = %v, want %v", got, tt.want)
			}
			if ok != tt.wantOk {
				t.Errorf("OptionString() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestStepManifest_OptionBool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		options map[string]any
		key     string
		want    bool
		wantOk  bool
	}{
		{
			name:    "nil options",
			options: nil,
			key:     "foo",
			want:    false,
			wantOk:  false,
		},
		{
			name:    "empty options",
			options: map[string]any{},
			key:     "foo",
			want:    false,
			wantOk:  false,
		},
		{
			name:    "key not found",
			options: map[string]any{"bar": true},
			key:     "foo",
			want:    false,
			wantOk:  false,
		},
		{
			name:    "value is true",
			options: map[string]any{"foo": true},
			key:     "foo",
			want:    true,
			wantOk:  true,
		},
		{
			name:    "value is false",
			options: map[string]any{"foo": false},
			key:     "foo",
			want:    false,
			wantOk:  true,
		},
		{
			name:    "value is not bool (string)",
			options: map[string]any{"foo": "true"},
			key:     "foo",
			want:    false,
			wantOk:  false,
		},
		{
			name:    "value is not bool (int)",
			options: map[string]any{"foo": 1},
			key:     "foo",
			want:    false,
			wantOk:  false,
		},
		{
			name:    "value is not bool (map)",
			options: map[string]any{"foo": map[string]any{"nested": true}},
			key:     "foo",
			want:    false,
			wantOk:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := StepManifest{
				ID:      types.StepID("test-step"),
				Name:    "Test Step",
				Image:   "test:latest",
				Inputs:  []StepInput{{Name: "test", MountPath: "/test", Mode: StepInputModeReadOnly, SnapshotCID: "cid"}},
				Options: tt.options,
			}
			got, ok := m.OptionBool(tt.key)
			if got != tt.want {
				t.Errorf("OptionBool() value = %v, want %v", got, tt.want)
			}
			if ok != tt.wantOk {
				t.Errorf("OptionBool() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}
