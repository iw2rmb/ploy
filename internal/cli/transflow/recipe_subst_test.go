package transflow

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestPreSubstituteRecipe_WritesPreFile(t *testing.T) {
    dir := t.TempDir()
    in := filepath.Join(dir, "orw_apply.rendered.hcl")
    content := "recipe_class=\"${RECIPE_CLASS}\"\ncoords=\"${RECIPE_COORDS}\"\ntimeout=\"${RECIPE_TIMEOUT}\"\n"
    if err := os.WriteFile(in, []byte(content), 0644); err != nil {
        t.Fatal(err)
    }
    pre, err := preSubstituteRecipe(in, "com.acme.MyRecipe", "org.example:artifact:1.2.3", "15m")
    if err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
    b, err := os.ReadFile(pre)
    if err != nil { t.Fatalf("read pre: %v", err) }
    s := string(b)
    if want := "com.acme.MyRecipe"; !strings.Contains(s, want) { t.Fatalf("missing %q in pre", want) }
    if want := "org.example:artifact:1.2.3"; !strings.Contains(s, want) { t.Fatalf("missing %q in pre", want) }
    if want := "15m"; !strings.Contains(s, want) { t.Fatalf("missing %q in pre", want) }
}

