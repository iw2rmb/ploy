Great question. Let’s make your slicer incremental so it ingests diffs and updates its AST/indices on the fly—without reindexing the whole repo—and then talk about realistic memory footprints across languages.

⸻

What changes when you go incremental

1) Architecture tweaks (small but important)
	•	Document store with byte-accurate edits. Keep each file’s current buffer in a rope/piece-table (or just strings if files are small). You need byte offsets to forward edits into the parser.
	•	Per-file AST cache + symbol edges. Keep one lightweight tree per open/“hot” file and persist global symbol/edge facts (defs, refs, calls, imports) in SQLite.
	•	Reverse-deps index. For any symbol S, you must quickly find “who might be affected if S changes?” (callers/importers/implementers).
	•	Work queue + debounce. Batch bursts of edits (e.g., save or multi-file patch) into a single pipeline run, then fan out dependent updates.

This preserves your tiny footprint: only a handful of ASTs in RAM at any time; global graph facts stay in SQLite/Tantivy mmap files.

⸻

2) Diff-to-AST update pipeline (per file)

Below is the exact control flow for Tree-sitter (but the idea generalizes):
	1.	Receive a diff → convert to one or more edits (start_byte, old_end_byte, new_end_byte).
	2.	Apply edit to the existing tree via ts_tree_edit (or the language binding’s equivalent). This shifts node ranges but doesn’t reparse yet.  ￼ ￼
	3.	Incremental reparse: call ts_parser_parse(parser, old_tree, new_buffer)—Tree-sitter reparses only the affected region, using the old tree as a guide.  ￼ ￼
	4.	Find changed subtrees: Tree-sitter lets you walk “changed ranges” to re-extract just the impacted symbols/edges (defs/refs/calls) and update SQLite rows for overlapping spans.
	5.	Reindex text for the changed file in Tantivy (delete-by-id + add). Tantivy’s mmap keeps RSS low.
	6.	(Optional) Embeddings: recompute vectors only for changed symbols/snippets; update/delete in HNSW.

Why Tree-sitter? It’s designed to “efficiently update the syntax tree as the source file is edited,” i.e., this exact workflow.  ￼ ￼

For other ecosystems
	•	Roslyn (C#) uses immutable red/green trees. On edits, it rebuilds only the affected green nodes; red nodes are lazily rewrapped—great for low-latency incremental updates. (O(log n) node churn typical.)  ￼ ￼
	•	Clang/libclang (C/C++) doesn’t do fine-grained incremental AST across the whole TU; you typically reparse the TU (with precompiled preambles/modules to cut memory/time) and can query resource usage per TU.  ￼ ￼
	•	TypeScript keeps project state in tsserver; memory can be large on big repos (hundreds of MB–GB). If you’re footprint-sensitive, avoid running tsserver and just maintain your own lighter indices, calling the TS compiler only on demand.  ￼ ￼

⸻

3) Cross-file propagation (impact)

After updating a file’s AST/symbols:
	•	If the file exports a symbol whose name/signature changed, look up importers/callers via your reverse-deps index and schedule lightweight refresh:
	•	For Tree-sitter-only stacks, you can cheaply re-scan headers/imports textually and re-extract edges (no full type system).
	•	For typed languages that need semantic binding (C++/TS/Java), prefer two-tier: keep your fast structural graph in SQLite and defer heavyweight type checks to optional background tasks.
	•	Update embeddings/text indices for those dependents only if their outgoing edges changed (most won’t).

⸻

4) Incremental slice algorithm (change-driven)

The original (prompt-driven) slicer becomes change-driven when you have a diff:
	1.	Seeds = changed symbols (defs, signatures, touched methods/classes) derived from the changed subtrees.
	2.	Expand one ring of callees/referenced types/configs, and two rings of callers/tests, using your stored graph (weighted BFS with fan-out caps).
	3.	Add prior context the LLM saw last time for continuity (if cached).
	4.	Token pack with the same priority order (anchors → types → representative callers → tests).
	5.	Stop early if edits are localized and no outward edges changed.

This is classic change impact analysis applied to LLM context selection.

⸻

5) Memory-savvy policies
	•	LRU AST cache: keep trees only for the N most-recently edited/anchored files (e.g., 50–200).
	•	Store symbols/edges only for cold files (SQLite rows + offsets).
	•	String interning for identifiers; content hashing to skip reindex when the body didn’t really change.
	•	Feature flags: run embeddings only when the natural-language prompt lacks code names; skip entirely for scripted edits.

⸻

Pseudocode: incremental update

def apply_patch(repo_state, patch):
    for file, edits in patch.group_by_file():
        buf = repo_state.buffer[file]
        buf.apply(edits)                             # piece table/rope updates

        old_tree = repo_state.trees.get(file)
        if old_tree:
            for e in edits:
                old_tree.edit(to_ts_input_edit(e))   # shift ranges
        new_tree = ts_parser.parse(buf.text, old_tree)

        changed_ranges = get_changed_ranges(old_tree, new_tree)
        changed_symbols, changed_edges = extract_symbols_edges(new_tree, changed_ranges)

        sqlite.begin()
        sqlite.delete_rows(file, span_overlaps=changed_ranges)
        sqlite.insert_symbols(changed_symbols)
        sqlite.insert_edges(changed_edges)
        sqlite.commit()

        tantivy.replace_document(file, buf.text)     # BM25
        if embeddings_on:
            for s in changed_symbols:
                ann.upsert(s.id, embed(s.snippet))

        repo_state.trees[file] = new_tree

    # Cross-file invalidations:
    impacted = reverse_dep_lookup(changed_symbols_exported(changed_symbols))
    schedule_light_refresh(impacted)  # re-extract edges or reparse if needed

def build_slice_for_llm(diff, budget):
    seeds = symbols_from_diff(diff)                   # changed defs/refs
    frontier = weighted_bfs(seeds, graph=sqlite_graph, max_hops=(1,2))
    nodes   = fine_grain_slice_if_large(seeds + frontier)
    return token_pack(nodes, budget)


⸻

Realistic AST/CST memory: what to expect

Numbers vary wildly by language, grammar, and whether the tree is concrete (every token) or typed/semantic (Clang/Roslyn). Here’s a practical view from public docs/issues and tool authors; treat these as order-of-magnitude guidance, not hard limits:

Family	Typical in-RAM shape	Ballpark footprint notes
Tree-sitter (CST, per file)	Concrete tree of tokens + lightweight nodes	Designed for incremental per-file parsing; trees are relatively compact and fast to update. Expect a few × the file size in memory per open tree; keep only hot files in RAM.  ￼ ￼
Roslyn (C#)	Immutable green trees + lazy red wrappers	Incremental edits rebuild only affected green nodes; red nodes are ephemeral. Powerful but can be memory-expensive on large solutions (multi-GB IDE sessions are not unusual).  ￼ ￼ ￼
Clang (C/C++)	Rich, typed AST per translation unit	TUs can reach tens to hundreds of MB (JSON dumps even larger). Use precompiled preambles/modules to reduce peak memory and reparse cost.  ￼ ￼
TypeScript compiler/tsserver	AST + binder + type checker graphs	On large monorepos, ~1–1.5 GB RSS for the server is reported; this includes types/services, not just AST. For a lean tool, avoid keeping tsserver resident.  ￼ ￼
Python	ast (lossy AST) vs LibCST (lossless CST)	ast is small but loses comments/formatting. LibCST retains all trivia, so it’s larger; keep few trees resident and parse on demand.  ￼ ￼

If you must quantify: CSTs (Tree-sitter/LibCST) often land in the ~2–6× source bytes range per open file; typed ASTs (Clang/Roslyn) can be much larger because they include types, templates/generics, symbol tables, and side tables. These are heuristic ranges; measure on your repo.

For Clang, you can even query memory of AST side tables and nodes programmatically (ASTContext::getASTAllocatedMemory, getSideTableAllocatedMemory), and libclang exposes per-TU resource breakdowns.  ￼

⸻

Practical knobs for low RAM
	•	Keep N trees open (e.g., 100) via LRU; evict oldest to just symbols/edges.
	•	Demote “utility” subtrees (logging/collections) after first inclusion in a slice.
	•	Precompiled preambles / modules (C++), project references (TS), solution filters (Roslyn) to narrow what’s loaded.  ￼
	•	Mmap indices (Tantivy) + SQLite on disk; both keep RSS predictable.
	•	Quantized embeddings only if the prompt lacks code names; otherwise rely on BM25+AST edges.

⸻

Edge cases & gotchas
	•	Renames/moves: Treat as delete+add; keep a path alias table for a few commits so reverse-deps can still find “old path → new path.”
	•	Large generated files: Don’t AST-parse them; index textually with a cap or ignore by pattern.
	•	Header-heavy C++: Prefer indexing headers structurally with Tree-sitter (imports/macros) and run Clang only for focused tasks; use precompiled preambles on repeat reparses.  ￼
	•	TS monorepos: Avoid long-running tsserver if footprint is critical; fall back to BM25+Tree-sitter CST + selective tsc invocations.  ￼

⸻

TL;DR upgrade plan
	1.	Wire ts_tree_edit → incremental reparse → changed-range extraction.  ￼
	2.	Update SQLite edges and Tantivy docs only for changed ranges/files.
	3.	Propagate via reverse-deps to a small set of dependents.
	4.	Make the slicer change-driven when diffs are present (seeds = changed symbols).
	5.	Keep RAM small with LRU AST cache, mmap indices, and optional embeddings.
