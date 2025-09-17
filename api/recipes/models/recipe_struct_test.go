package models

import (
	"reflect"
	"testing"
)

func TestRecipeStructDoesNotExposeValidationField(t *testing.T) {
	typeOfRecipe := reflect.TypeOf(Recipe{})
	if _, ok := typeOfRecipe.FieldByName("Validation"); ok {
		t.Fatalf("expected Recipe struct to not expose Validation field")
	}
}
