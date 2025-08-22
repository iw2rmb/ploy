Great point. Real code is often in a “red” state—half-typed, merge-conflicted, or mid-refactor. Your slicer should keep working anyway. Here’s how to make the pipeline error-tolerant without blowing up CPU/RAM.

Core idea

Use a tolerant, incremental CST as your ground truth (e.g., Tree-sitter) and treat typed/semantic info as optional layers. You always have some tree—even with syntax errors—so you can still extract most symbols/edges and build a useful slice.

⸻

1) Parsing strategy (always produce a tree)
	•	Tree-sitter (recommended): it never fails hard; it yields a tree with ERROR nodes covering invalid regions. You can:
	•	Extract symbols/edges from valid subtrees.
	•	Skip or downgrade facts originating inside/through ERROR nodes.
	•	Other ecosystems with error recovery:
	•	Roslyn (C#) creates a full syntax tree with “missing” tokens + diagnostics.
	•	TS/JS (Babel/TS) have error-recovery modes; keep them behind a feature flag if you use them.
	•	Clang (C/C++): treat it as a semantic layer; don’t rely on it for always-on parsing.

Rule: the CST is authoritative for structure; semantic layers are best-effort and can be bypassed.

⸻

2) Incremental update flow with errors

When a diff arrives:
	1.	Apply edits to the old CST (tree-edit) and incrementally reparse.
	2.	Discover changed ranges. For each range:
	•	If no ERROR in the new tree → extract/refresh symbols + edges normally.
	•	If ERROR present:
	•	Keep last-known-good (LKG) symbols/edges that don’t overlap the changed span.
	•	Try structural salvage inside the bad region:
	•	Cheap token scans (balanced braces/indent heuristics) to recover enclosing definition boundaries.
	•	Conservative stubs for signatures you can still see (e.g., fn name(args)... without a full body).
	3.	Mark facts with trust levels:
	•	high: from clean subtrees,
	•	low: crosses or comes from an ERROR region,
	•	stale: LKG fact from before the edit touching the same span.

This lets downstream ranking prefer clean facts but still operate when things are broken.

⸻

3) Building slices when code is broken

Anchor selection
	•	Prefer anchors from clean subtrees; if the user’s change clearly targets a broken file, you can still anchor on the enclosing definition (found via salvage) and nearby clean callees/callers.

Expansion
	•	Traverse graph edges but stop expansion through low-trust nodes unless you must (e.g., no other path exists).
	•	Boost callers/tests/configs that sit outside error regions to give the LLM stable context.

Packaging
	•	For broken definitions:
	•	Include the signature + stable surrounding lines (e.g., ±40) rather than the whole broken body.
	•	Add a small “state notes” block: “This region fails to parse; adjacent helpers and callers included.”
	•	If you have an LKG version, include a compact diff (old → new intent) so the model sees the intended shape.

⸻

4) Indexing & search under errors
	•	Text/BM25 index (Tantivy): unaffected; you can still retrieve candidates from identifiers, comments, paths.
	•	Symbol store (SQLite):
	•	Upsert clean symbols normally.
	•	For ERROR regions, write stub rows (name, kind, span) flagged low_trust.
	•	Do not delete LKG rows unless you’re sure the symbol was removed; instead, flip them to stale so reverse-deps still find something.
	•	Embeddings (optional):
	•	Skip vectors for broken snippets; store the last valid vector (stale) so semantic retrieval still lands nearby.

⸻

5) Cross-file impact with broken sources
	•	If an exported symbol’s name/signature can’t be confidently read, issue a narrow invalidation: re-scan importers textually (fast grep) but defer heavy recomputation until the file is clean.
	•	For typed languages, generate minimal stubs for public surfaces (interfaces, function sigs) from LKG so call sites remain navigable.

⸻

6) Edit loop hardening (when the LLM proposes a patch)
	•	Apply patch → reparse. If still broken:
	•	Auto-format (black/prettier/gofmt) and retry parse.
	•	Run micro-fixups: add missing braces/parentheses/end, restore imports the model removed.
	•	If unrecoverable, roll back that file to LKG and keep the rest of the patch (partial commit strategy), or produce a follow-up prompt that includes the parser diagnostics.
	•	Gate “ready to run tests” on no-ERROR in touched files.

⸻

7) Minimal extra data you’ll track
	•	trust_level on every symbol/edge row: {high, low, stale}.
	•	has_error_nodes per file + byte ranges of ERROR spans.
	•	lkg_hash per file to quickly detect when you can safely drop stale facts.

⸻

8) Pseudocode (error-aware extraction)

def extract_symbols_edges(tree, changed_ranges):
    out = []
    for r in changed_ranges:
        subtree = subtree_for_range(tree, r)
        if contains_error(subtree):
            out += salvage_stubs(subtree, trust="low")
            continue
        out += precise_extract(subtree, trust="high")
    return out

def expand_graph(anchors):
    q = PriorityQueue()
    for a in anchors: q.push(a, priority=score(a))
    seen = set()
    while not q.empty():
        n = q.pop()
        if n in seen: continue
        seen.add(n)
        yield n
        for e in graph.edges(n):
            if e.trust == "low": continue        # skip through broken regions
            if degree(e.dst) > HIGH_FANOUT: continue
            q.push(e.dst, priority=score(e.dst))


⸻

9) How this affects CPU/RAM
	•	No big cost increase: you’re already keeping only a small LRU of CSTs in RAM. Error handling just adds:
	•	A few byte ranges per file,
	•	A trust_level column in SQLite,
	•	Occasional stub generation (cheap string ops).
	•	You avoid firing heavy compilers on every keystroke; semantic passes remain opt-in.

⸻

10) Practical heuristics (copy/paste)
	•	Treat a file as poisoned if >15% of its bytes are under ERROR nodes; avoid anchoring there unless unavoidable.
	•	Keep 1 LKG snapshot per poisoned file (content hash + extracted symbols).
	•	In slices, never include half a token/statement; escalate to enclosing node or summarize.
	•	Show the model one good caller and one good callee outside the error region so it can reason even when the target is broken.

⸻

Bottom line

By rooting the pipeline in a tolerant CST (Tree-sitter/Roslyn-style), layering trust on facts, and falling back to stubs + LKG when needed, the slicer keeps producing precise, runnable context—even when parts of the repo don’t parse. You don’t pay a big CPU/RAM tax, and you don’t block developer flow on transient syntax errors.