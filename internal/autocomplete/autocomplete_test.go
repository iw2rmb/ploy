package autocomplete

import (
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/clitree"
)

func TestJoinPathAndDescription(t *testing.T) {
	if got := joinPath("a", "b", "c"); got != "a b c" {
		t.Fatalf("joinPath()=%q want 'a b c'", got)
	}

	// description precedence
	n := clitree.Node{Synopsis: "syn", Description: "desc"}
	if d := nodeDescription(n); d != "syn - desc" {
		t.Fatalf("nodeDescription syn+desc=%q", d)
	}
	n = clitree.Node{Synopsis: "syn"}
	if d := nodeDescription(n); d != "syn" {
		t.Fatalf("nodeDescription syn only=%q", d)
	}
	n = clitree.Node{Description: "desc"}
	if d := nodeDescription(n); d != "desc" {
		t.Fatalf("nodeDescription desc only=%q", d)
	}
}

func TestHelpersOnSmallTree(t *testing.T) {
	nodes := []clitree.Node{{Name: "alpha", Synopsis: "a", Subcommands: []clitree.Node{{Name: "one"}, {Name: "two"}}}, {Name: "beta"}}
	names := topLevelNames(nodes)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("topLevelNames=%v", names)
	}
	pairs := nodePairs(nodes)
	if len(pairs) != 2 || !strings.HasPrefix(pairs[0], "alpha:") {
		t.Fatalf("nodePairs=%v", pairs)
	}
	kids := childNames(nodes[0].Subcommands)
	if len(kids) != 2 || kids[0] != "one" || kids[1] != "two" {
		t.Fatalf("childNames=%v", kids)
	}
	cm := buildChildren(nodes)
	if len(cm[""]) != 2 { // root path children
		t.Fatalf("buildChildren root children=%v", cm[""])
	}
	if got := cm["alpha one"]; len(got) != 0 {
		t.Fatalf("expected no grandchildren for alpha one, got=%v", got)
	}
}

func TestGenerateScriptsContainHeaders(t *testing.T) {
	nodes := clitree.Tree()
	bash := GenerateBash(nodes)
	if !strings.Contains(bash, "_ploy_completions()") {
		t.Fatalf("bash script missing function header")
	}
	zsh := GenerateZsh(nodes)
	if !strings.Contains(zsh, "#compdef ploy") {
		t.Fatalf("zsh script missing compdef header")
	}
	fish := GenerateFish(nodes)
	if !strings.Contains(fish, "complete -c ploy") {
		t.Fatalf("fish script missing complete header")
	}
	all := GenerateAll()
	if all["bash"] == "" || all["zsh"] == "" || all["fish"] == "" {
		t.Fatalf("GenerateAll returned empty entries: %v", all)
	}
}
