package contracts

import "testing"

func TestNormalizeStackExpectation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   *StackExpectation
		want *StackExpectation
	}{
		{name: "nil"},
		{name: "empty", in: &StackExpectation{}},
		{name: "whitespace", in: &StackExpectation{Language: " ", Tool: "\t", Release: "\n"}},
		{name: "trimmed", in: &StackExpectation{Language: " java ", Tool: " maven ", Release: " 17 "}, want: &StackExpectation{Language: "java", Tool: "maven", Release: "17"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeStackExpectation(tt.in)
			if got == nil || tt.want == nil {
				if got != tt.want {
					t.Fatalf("NormalizeStackExpectation() = %+v, want %+v", got, tt.want)
				}
				return
			}
			if !got.Equal(*tt.want) {
				t.Fatalf("NormalizeStackExpectation() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
