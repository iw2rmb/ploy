Using ASTs (and richer code graphs) to automatically “fence” the LLM’s context around a change request is both sensible and increasingly common. You can get much tighter, cheaper prompts by selecting only the semantically related code (callers, callees, implementers, referenced types, configs, and tests) instead of whole files or fuzzy text chunks.

Below is a concise research-backed plan, comparisons, and a concrete algorithm you can implement.

What you’re trying to do

Given a target element (e.g., “rewrite class Foo” or “replace method bar()”), automatically construct a minimal, sufficient context for the LLM that:
	1.	preserves compile-time and behavioral dependencies,
	2.	includes nearby tests and configs,
	3.	fits a token budget,
	4.	stays fast and incremental as the repo evolves.

Foundations to rely on (AST → Graph → Slice)
	•	Program slicing / impact analysis. Classic SE tools compute the set of statements/elements that could affect or be affected by a slicing criterion (your class/method). Variants include static, dynamic, and thin slicing, typically via dependence graphs (data + control) and call graphs. This is exactly the “what are the borders?” question.  ￼ ￼ ￼ ￼
	•	Language-server/precise indices for references. LSP/SCIP/LSIF give compiler-accurate find references / go to definition across repos; you can query call hierarchies and symbol usages to build the dependency frontier fast.  ￼ ￼ ￼ ￼
	•	AST-based match/transform tools. Tree-sitter (fast incremental CST/AST), Semgrep/ast-grep (pattern + data flow), and refactoring frameworks (OpenRewrite, Spoon, Clang tooling, Rope/Bowler/LibCST) let you detect/transform relevant code safely.  ￼ ￼ ￼ ￼ ￼
	•	Heavier semantic analysis when needed. CodeQL databases provide AST + control/data-flow + call graphs for deep impact queries (not for rewriting, but great for deciding context).  ￼
	•	Analogous practice: Test Impact Analysis (TIA) uses dependency maps to run only impacted tests—your context selection can mirror this.  ￼

Who already does parts of this
	•	Sourcegraph Cody builds a precise code graph (SCIP/LSIF) and uses it to fetch just-in-time repo context for chat/edits; it’s designed for large, multi-repo code bases. This is the closest production blueprint.  ￼
	•	Academic & open research on repo-level RAG for code (GraphCoder, RepoHyper, CodeRAG, knowledge-graph approaches) shows that graph-based retrieval beats plain embeddings for picking useful context. Use these ideas even if you don’t adopt the models.  ￼ ￼ ￼
	•	Refactoring frameworks (OpenRewrite for Java, Spoon/Clang/Rope/Bowler/LibCST) don’t do RAG, but they do give robust AST edits and can feed “what changed” and “what else is touched” into your retriever.  ￼ ￼ ￼ ￼
	•	Semgrep/ast-grep are pragmatic for pattern-based selection or quick codemods, now with AST-printing autofix; they can seed your “impact frontier” cheaply across many languages.  ￼ ￼

A concrete algorithm for “context borders”

Think: seed → resolve → expand → prune → package.
	1.	Seed the slice
	•	Identify the AST node(s) corresponding to the requested change (class, method, or symbol) using your language server or parser.
	•	Normalize to a symbol handle (fully qualified name, file + position, SCIP symbol, etc.).  ￼
	2.	Resolve precise references
	•	Query index for:
	•	Incoming edges: callers, subclasses/implementations, file/module imports, tests referencing the symbol.
	•	Outgoing edges: callees, referenced types, constants, config/resources.
	•	Use LSP/SCIP/LSIF findReferences, definition, call hierarchy; fall back to Tree-sitter-based grep where language servers are missing.  ￼ ￼ ￼
	3.	Build a multi-layer code graph
	•	Nodes: symbols (methods, classes, fields), modules/files, tests.
	•	Edges: calls, inheritance/implements, import/exports, data-flow deps (optional for precision), test-to-code links (from TIA or previous coverage).
	•	Optionally enrich with CodeQL data/control-flow for tricky languages (security-sensitive or reflection-heavy sites).  ￼
	4.	Expand to a bounded frontier
	•	Run a personalized PageRank or k-step BFS from the seed with weights:
	•	Prefer intra-file > intra-module > cross-module,
	•	Boost direct callers/callees and interface contracts,
	•	Boost relevant tests and build files,
	•	De-emphasize high fan-out utility code and auto-generated files.
	•	Cap by tokens (estimate tokens per node), not by files.
	5.	Optionally compute slices (fine-grain)
	•	For big functions, compute backward/forward static slices on variables/types impacted by your change to extract just the statements that matter. (Dynamic slices if you have coverage traces.)  ￼ ￼
	6.	Prune & dedupe
	•	Collapse SCCs, cut rarely relevant edges (e.g., logging wrappers), and limit depth until you fit your budget.
	•	Keep minimal compilable units (e.g., import headers, enclosing type definitions) so the LLM sees enough to compile mentally.
	7.	Package the prompt
	•	Structure as: (a) Change brief you write, (b) Seed code, (c) Direct deps (interfaces/types/callees), (d) Callers/tests, (e) Build/config.
	•	Include symbol graph snippets (lightweight adjacency lists) so the LLM understands relationships.
	8.	Feedback & caching
	•	Cache indices (SCIP/LSP) and graph partitions; incremental update on each commit for speed.  ￼

This pipeline gives you aligned, focused context and keeps costs down because expansion is graph-guided instead of “nearest-embedding chunks.”

Implementation building blocks (mix & match)
	•	Index & resolve
	•	Precise indices: SCIP/LSIF indexers for Python/Java/TS/Go/C++ (e.g., scip-java, scip-python, scip-clang).  ￼ ￼ ￼
	•	Language servers: LSP/tsserver/JDT/Pyright for cross-refs and call hierarchies.  ￼ ￼
	•	Tree-sitter: fast incremental parsing when servers are unavailable.  ￼
	•	Deep analysis (optional)
	•	CodeQL DBs for per-language AST+CFG+DFG+call graph queries to refine impact. (Note: CodeQL doesn’t edit code.)  ￼ ￼
	•	Rewrite/codemod
	•	OpenRewrite (Java), Spoon (Java), Clang tooling (C/C++), LibCST/Bowler/Rope (Python) for safe edits after the LLM proposes a patch.  ￼ ￼ ￼ ￼
	•	Semgrep/ast-grep for broad, rule-driven selections or templated fixes (and to seed graph expansion).  ￼
	•	Analogy & tests
	•	TIA logic to prioritise impacted tests as part of context.  ￼

How this compares to common alternatives
	•	Plain embedding RAG on code chunks: fast to build but weak on semantic dependencies (callers/implementers), tends to miss tests/config; graph-guided retrieval is more precise for repo-level tasks. (See repo-level RAG papers.)  ￼ ￼ ￼
	•	Search/grep only: very fast but brittle; misses cross-module and type-driven links.
	•	“Send the whole file(s)”: simple but expensive; hurts latency and token costs and can dilute the LLM’s focus.

Practical heuristics that work well
	•	Two-ring context: (1) Seed + direct callers/callees + referenced types; (2) tests + configs + build rules that reference ring-1 nodes.
	•	Edge weighting: interface/trait contracts > inheritance > imports > sibling file proximity > historical co-change (git) > textual similarity.
	•	Fan-out control: downsample utility libs (logging, collections) after the first hop.
	•	Token guardrails: keep 60–70% of budget for seed + interfaces + direct deps; use the rest for callers/tests.

Quick blueprint (pseudo-ish)

input: change_target (symbol), token_budget B
S ← resolve_symbol(change_target)                  // LSP/SCIP
G ← build_or_load_graph()                         // symbols, files, calls, imports, tests
F1 ← neighbors(S, types|callees|defs|implementers|imports) 
F2 ← callers(S) ∪ tests_referencing(S or F1)
R  ← rank(F1 ∪ F2, weights=call_edges>types>imports, demote=high_fanout)
C  ← slices_for_big_nodes(R, strategy=backward/forward on impacted vars) // optional
P  ← pack([S, F1, C, F2, configs/build], budget=B, ensure-min-compilable-units=true)
return prompt(P)

Language-specific notes
	•	TypeScript/JS: tsserver findReferences / call hierarchy are excellent; project references help scale.  ￼ ￼
	•	Java: generate SCIP via scip-java or use JDT; for refactors, pair with OpenRewrite/Spoon.  ￼ ￼
	•	Python: Pyright/Jedi + scip-python; safe edits via LibCST/Bowler/Rope.  ￼ ￼
	•	C/C++: scip-clang or clang-query/matchers if you need custom AST matching.  ￼ ￼

Metrics to track
	•	Context recall (did we include all compile-time deps and top N runtime deps?),
	•	Token cost (avg tokens per change),
	•	Latency (index lookup + packing),
	•	Outcome (build/tests pass after LLM patch),
	•	Human review time (PR discussion length, change risk score). (Recent work even computes PR-risk from call graphs + history—use as a gating signal.)  ￼

Bottom line

Your idea makes strong sense. The winning recipe in practice is AST + symbol/usage indices + light graph expansion (optionally augmented by slices)—not embeddings alone. If you stand up SCIP/LSIF (or language servers) and add a small, weighted graph expansion with a token-aware packer, you’ll get focused, repeatable LLM prompts that are faster, cheaper, and safer than file-dumping.
