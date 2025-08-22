Great question. Here’s a crisp, engineering-oriented comparison of implementing your incremental AST + context-slicing tool in Go, Haskell, C, or “Zed”—plus a few “niche” options and whether GPUs ever help.

⸻

Go

Pros
	•	Easy concurrency (indexing, parsing, packing run as goroutines).
	•	Mature, portable static binaries; good ops story (pprof, trace).
	•	Solid native BM25 options (Bleve/Bluge)—no JVM required.  ￼ ￼
	•	First-class C FFI via cgo for Tree-sitter/SQLite/ONNX Runtime; per-call overhead is small on modern Go.  ￼
	•	Tree-sitter has a stable C API—easy to bind.  ￼

Cons
	•	GC adds some tail memory vs. C/Zig.
	•	If you depend on Tantivy (Rust), you’ll cross languages; prefer Bleve/Bluge in Go to keep footprint simple. (Tantivy: BM25 + mmap if you do embed Rust.)  ￼

Verdict: Great “small ops” choice. Pair Go + Tree-sitter C API + Bleve + SQLite for a compact, fast stack.

⸻

Haskell

Pros
	•	Superb for compiler-ish code (algebraic data types, persistent trees).
	•	Proven for static analyzers (Facebook Infer, Semgrep).  ￼ ￼

Cons
	•	GHC runtime (RTS) is non-trivial; memory tuning often required (e.g., GHCRTS=-M...).  ￼ ￼
	•	Smaller hiring pool + fewer ready-made libs for BM25/ANN.
	•	FFI workable but more friction than Go/C.

Verdict: Excellent correctness and analysis ergonomics; not the first pick if your top goal is minimal RSS and simplest ops.

⸻

C

Pros
	•	Smallest possible runtime and memory; direct use of Tree-sitter’s C API and SQLite.  ￼
	•	Zero GC overhead; tight control over caches, arenas, and mmaps.

Cons
	•	Implementation speed and safety risk; you’ll hand-roll utilities (thread pools, serializers).
	•	Fewer off-the-shelf IR/search libs (you’d call C++/Rust for fancy bits or keep it simple with BM25-lite).

Verdict: Best raw footprint/perf. Pick C if you want a tiny resident indexer/daemon and you’re comfortable managing memory and FFI.

⸻

Niche languages that shine on footprint/perf
	•	Zig — no GC, tiny static binaries, great C interop, explicit allocators; good fit for a compact, single-binary indexer. Ecosystem smaller (you’ll wrap C libs for BM25/ANN/ONNX).  ￼
	•	OCaml — battle-tested for analyzers (Infer, Semgrep). Footprint > C/Zig but strong parsing/AST ergonomics and solid performance when tuned.  ￼ ￼
	•	Nim — compiles to C, small static binaries; pragmatic alternative to Zig/C if your team prefers Python-like syntax. Ecosystem smaller; you’ll still wrap C libs.  ￼


⸻

Practical picks (smallest footprint per ecosystem)
	•	Go: Tree-sitter C API + Bleve/Bluge (BM25) + SQLite; optional ONNX Runtime (C API via cgo) for tiny embeddings.  ￼ ￼ ￼
	•	C: Tree-sitter + SQLite + a compact BM25 index (or bind Xapian/Tantivy via C); tiny and fast.  ￼
	•	Zig: Call Tree-sitter/SQLite/ONNX directly via C ABI; write your own light BM25 or wrap a C lib.  ￼
	•	Haskell: Great analyzer core; pair with a small C/Rust sidecar for search if RSS matters.  ￼

⸻

My recommendation
	•	If your top goals are small RSS + fast dev + easy ops: Go implementation (Bleve + SQLite + Tree-sitter C API).
	•	If you want smallest possible binary and control (and are okay with lower-level dev): C (or Zig) core with Tree-sitter/SQLite.
	•	If your team is analysis-heavy and wants algebraic rigor: Haskell core with a slim C/Go helper for search/IO.
