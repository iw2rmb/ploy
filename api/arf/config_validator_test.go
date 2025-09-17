package arf

import "testing"

func TestInitializeValidatorReturnsNil(t *testing.T) {
	cfg := DefaultConfig()
	if validator := cfg.InitializeValidator(); validator != nil {
		t.Fatalf("expected no recipe validator, got %T", validator)
	}
}
