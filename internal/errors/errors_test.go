package errors

import "testing"

func TestConstructorsAndFrom(t *testing.T) {
	e := NotFound("missing", map[string]string{"k": "v"})
	if e.HTTPStatus != 404 || e.Code != CodeNotFound {
		t.Fatalf("unexpected mapping: %+v", e)
	}
	if From(e).Code != CodeNotFound {
		t.Fatalf("from not typed")
	}
	if From(nil) != nil {
		t.Fatalf("from nil should be nil")
	}
}

func TestValidateNotEmpty(t *testing.T) {
	if ValidateNotEmpty("name", "ok") != nil {
		t.Fatalf("unexpected error")
	}
	if err := ValidateNotEmpty("name", ""); err == nil || err.Code != CodeValidation {
		t.Fatalf("expected validation error")
	}
}
