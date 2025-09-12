package transflow

import "testing"

func TestRunIDHelpersContainIDs(t *testing.T) {
    if id := PlannerRunID("wf1"); id == "" || !strContains(id, "wf1-") {
        t.Fatalf("bad planner id: %s", id)
    }
    if id := ReducerRunID("plan1"); id == "" || !strContains(id, "plan1-") {
        t.Fatalf("bad reducer id: %s", id)
    }
    if id := LLMRunID("b1"); id == "" || !strContains(id, "llm-exec-b1-") {
        t.Fatalf("bad llm id: %s", id)
    }
    if id := ORWRunID("s1"); id == "" || !strContains(id, "orw-apply-s1-") {
        t.Fatalf("bad orw id: %s", id)
    }
}

func strContains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || (len(s) > len(sub) && (s[0:len(sub)] == sub || strContains(s[1:], sub)))) }
