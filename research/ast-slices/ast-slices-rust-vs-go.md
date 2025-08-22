Here’s a crisp, decision-ready comparison of building your incremental AST + graph-guided slicer in Go vs. Rust.

Executive summary
	•	Rust: best raw speed and lowest RSS; tighter integration with Tree-sitter and Tantivy; steeper dev curve.
	•	Go: fastest to ship and operate; great concurrency & tooling; a bit more RAM and slightly lower peak throughput.

⸻

Side-by-side (what matters for your tool)

Dimension	Rust	Go
Overall complexity	Higher: ownership/borrowing, lifetimes, more verbose error handling; pays off with safety.	Lower: simple syntax, batteries-included stdlib, easy concurrency; faster team onboarding.
Parsing / AST	First-class Tree-sitter crates (no FFI needed), excellent performance and incremental updates.	Tree-sitter via cgo; most heavy lifting is still in C so it’s fast, but you cross the cgo boundary.
Text search (BM25)	Tantivy is native Rust, SIMD-optimized, mmap-friendly; top-tier perf.	Bleve/Bluge are solid, pure Go; generally a notch slower than Tantivy for large corpora.
Graph + DB	rusqlite + mmap = tiny, fast; great zero-copy patterns.	go-sqlite3 is mature; simple to use; slightly higher allocs under load.
ANN / embeddings	Strong native libs (e.g., hnsw_rs) and good ONNX bindings; easy to keep CPU-only & quantized.	Pure-Go HNSW exists; often you’ll call C/C++ (hnswlib) via cgo. ONNX Runtime works via C API (cgo).
Concurrency model	Rayon/Tokio give high throughput; more design work to avoid contention.	Goroutines/channels = effortless concurrency; easier to keep pipelines saturated.
Binary size	Small static binaries (often single-digit MB unless you pull in big native deps).	Static binaries typically larger than Rust, but still “ops-friendly”.
Runtime overhead	No GC → predictable latency; lower steady-state RSS.	GC adds a little tail memory and rare pauses (usually tiny post-Go1.20), but ops are simple.
Cold start & mmap	Excellent; Rust + Tantivy/SQLite leverage OS page cache well.	Also good; Bleve’s segments mmap fine, just not as lean as Tantivy at scale.
Dev-ex & tooling	Cargo, Clippy, great test infra; steeper learning curve.	go build/test, pprof/trace, cross-compilation = dreamy for platform work.
Team/hiring	Smaller pool; Rust expertise required for velocity.	Larger pool; easy to ramp new contributors.


⸻

Performance & memory (realistic expectations)

These are ballpark patterns for repo-sized workloads (tens of thousands of files). Always benchmark on your codebase.
	•	Parsing / incremental AST
	•	Rust: tree-sitter crates avoid FFI; very low per-edit overhead.
	•	Go: cgo overhead exists but the parser work is in C; in practice, throughput is similar unless you call into the parser excessively per node.
	•	Search / indexing
	•	Rust: Tantivy usually wins on large collections (higher QPS, lower p99) and keeps RSS lower via mmap + tight data layouts.
	•	Go: Bleve/Bluge are plenty fast for medium repos; expect slightly higher CPU and RAM for equivalent recall.
	•	Memory footprint (RSS while serving)
	•	Rust stack (Tree-sitter crates + Tantivy + rusqlite + LRU of ~50–100 ASTs): commonly ~60–150 MB.
	•	Go stack (Tree-sitter via cgo + Bleve + go-sqlite3, same AST cache): commonly ~90–220 MB.
	•	Turning on embeddings + HNSW adds tens of MB in either language; keep vectors INT8/FP16 and HNSW M small to cap growth.
	•	Latency stability
	•	Rust: no GC → extremely predictable; great for bursty “update → slice” loops.
	•	Go: modern GC is excellent; occasional short pauses are usually inconsequential for a CLI/daemon like this.

⸻

Complexity & build considerations

Rust
	•	Pros: zero-cost abstractions, memory-safe concurrency, direct use of Tantivy and tree-sitter crates; fewer moving parts (no sidecars).
	•	Cons: longer ramp-up; more boilerplate around lifetimes & unsafe blocks (when interfacing with C); async vs sync choices add design surface.

Go
	•	Pros: quickest to get to a polished CLI/daemon; delightful profiling (pprof) and cross-platform builds; cgo to Tree-sitter/ONNX is straightforward.
	•	Cons: you’ll likely accept Bleve/Bluge (not Tantivy) or run a Rust search sidecar; slightly higher memory due to GC and allocations.

⸻

When to pick which

Choose Rust if…
	•	Minimizing RSS and maximizing throughput are top priorities.
	•	You want Tantivy and tree-sitter as first-class, in-process libraries.
	•	Your team is comfortable with Rust or you can invest in the learning curve.

Choose Go if…
	•	You want the simplest implementation path and ops story (single binary, goroutines, pprof).
	•	Your team already ships Go services; a modestly higher RAM budget is acceptable.
	•	You’re fine with Bleve/Bluge (or are ok adding a Rust sidecar later just for search).

⸻

Practical recommendation (for your stated goals: “small CPU & RAM footprint”)
	•	Rust edges it: a pure-Rust core (tree-sitter crates + Tantivy + rusqlite + hnsw_rs/ONNX) will typically hit lower RSS and higher indexing/search throughput than the Go equivalent.
	•	If time-to-first-prototype matters more, build it in Go first (Tree-sitter via cgo + Bleve + SQLite). You can always swap in a Rust search sidecar or migrate hotspots later.