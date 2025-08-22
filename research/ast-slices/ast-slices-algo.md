Totally makes sense—and you can do it even when the prompt doesn’t name specific classes or functions. Here’s a simple-but-detailed algorithm you can implement. Think of it as: understand the ask → find anchors → grow a precise slice → pack it for the LLM.

⸻

Goal

Input: (a) code base, (b) natural-language change request like “add exponential backoff to API retries”.
Output: a code slice (symbols/files/snippets + tiny relationship map) that’s minimal but sufficient for the LLM to make the change safely.

⸻

What you precompute once (fast lookups)
	1.	Symbol index: every function/class/module with its fully-qualified name, file, and span.
	2.	AST + code graph: call graph, import/use graph, type/implements edges.
	3.	Text index: inverted index over identifiers, comments, docstrings, file paths, commit messages.
	4.	Embeddings index (optional but powerful): short summary vector per symbol/file (derived from code + docs).
	5.	(Optional) Test/coverage map: which tests touch which symbols.

These can be built with language servers / parsers and updated incrementally.

⸻

The runtime algorithm (step-by-step)

A) Understand the prompt (no names needed)
	1.	Normalize text: lowercase, lemmatize, strip stopwords.
	2.	Extract intent: classify the action (add / remove / rewrite / tighten / optimize) and target “concepts” (retry, backoff, logging, cache, auth, payment).
	3.	Expand vocabulary: add synonyms and library hints.
	•	Example: “retry” → {retry, backoff, exponential, retryPolicy, Resilience4j, Tenacity, OkHttp interceptors, axios retry, requests.adapters}.
	4.	Produce 6–12 search queries: a mix of keywords and short phrases, e.g.,
	•	“retry policy”, “backoff”, “exponential backoff”, “http client”, “axios interceptor”, “OkHttp interceptor”, “Resilience4j Retry”.

Tip: you can let a small LLM produce (intent, concepts, synonyms) in one shot from the raw prompt; it doesn’t need repo context yet.

B) Find anchor candidates

Run three cheap, complementary searches and then fuse results:
	1.	Keyword search over the text index (identifiers, comments, file paths, READMEs, configs, commit messages).
	2.	Semantic search over the embeddings index (retrieve top-K nearest symbols/files to the prompt text).
	3.	API heuristics: if concepts map to well-known frameworks/APIs, search for their known entry points (e.g., OkHttpClient, axios.create, requests.Session, Logger, cache, redis, resilience4j.retry).

Score each hit:

S_text  = BM25 / tf-idf score
S_emb   = cosine similarity
S_path  = filename/path match boost (e.g., http/, retry/, auth/, payment/)
S_api   = library/API keyword presence boost
S_hist  = co-change/commit-message match (optional)
Final score = w1*S_text + w2*S_emb + w3*S_path + w4*S_api + w5*S_hist

Pick top N (say 20–80) anchor symbols (functions/classes/configs). Cluster by directory/module and keep the best per cluster to avoid redundancy.

C) Choose the true anchors

From those candidates, keep the ones that look like gateways (central points where the change is most effective):
	•	High in-degree (many callers) or sits on request/response paths (HTTP client, DB gateway, logging wrapper).
	•	Has names matching concepts (e.g., retry, logger, cache, auth, payment).
	•	Owns configuration for that concern.

Keep K anchors (e.g., K=3–10).

D) Grow the slice around anchors (dependency-aware)

Use the code graph; do a short, weighted expansion:
	•	Ring 0: the anchor symbol(s) themselves (include full definition, enclosing type, imports).
	•	Ring 1: direct callees, referenced types, configs, constants, interfaces/traits the anchor implements/calls.
	•	Ring 2: callers (where anchors are used), plus tests that reference Ring 0/1.
	•	Stop early if you hit high fan-out utilities (logging wrappers, collections); add 1 representative then demote others.

A simple expansion works well:

BFS from anchors with edge weights:
  call edge (callee)          = weight 1.0
  type/reference edge         = 0.8
  import edge                  = 0.6
  caller edge                  = 0.9
  test->code edge              = 0.7 (but boost tests overall)
Demote nodes with very high degree (utilities) by ×0.3 after the first inclusion.

E) Fine-grain slicing (optional but great for large functions)

If a function is huge, don’t paste the whole thing. Compute a static slice:
	•	Backward slice from variables/constants/configs related to the concept (“logger”, “retryPolicy”, “httpClient”).
	•	Forward slice from where the behavior begins (e.g., request send) to where the outcome occurs (e.g., response handling).
This extracts only the statements/control needed for the change.

If you don’t have a slicer, a pragmatic fallback:
	•	Include the function signature + surrounding 30–80 lines, plus any helper methods it calls (summarized if long).

F) Fit the token budget (pick the best bits)

Estimate tokens for each node/snippet and greedily pack the highest-value content first:

Priority order:
	1.	Change brief (a short, explicit goal you generate from the prompt).
	2.	Anchors (Ring 0) with full definitions.
	3.	Interfaces/types/configs they depend on (Ring 1).
	4.	One or two representative callers that show usage patterns (Ring 2).
	5.	The most relevant tests and build/config snippets.
	6.	Everything else summarized (you can attach auto-summaries in 1–2 sentences per leftover node).

Heuristic objective:

value(node) =  α * FinalScore(node) 
             + β * centrality(node) 
             + γ * testRelevance(node)
cost(node)  = token_estimate(node)
Pick nodes maximizing value/cost until budget.

G) Sanity checks and packaging
	•	Missing symbols? If a node references a type that’s not in the pack, include its minimal declaration.
	•	Non-compilable snippet? Prefer whole definition blocks (function/class) to avoid half-snippets.
	•	Package for the LLM:
	1.	Task brief (what to change, constraints, examples).
	2.	Tiny graph view (adjacency list of the included symbols).
	3.	Code blocks in the priority order above.
	4.	Tests to update (names/paths).
	5.	Constraints (perf/security, style, frameworks).

⸻

Pseudocode (end-to-end)

def build_slice(prompt, budget_tokens):
    # A) interpret
    intent, concepts, synonyms = interpret_prompt(prompt)  # small LLM or rules
    queries = make_queries(concepts, synonyms)

    # B) candidate search
    text_hits = keyword_search(queries)
    emb_hits  = embedding_search(prompt)
    api_hits  = api_hints(concepts)  # framework-specific names
    candidates = fuse_and_score(text_hits, emb_hits, api_hits)

    # C) select anchors
    anchors = pick_gateways(candidates, graph_metrics=['in_degree','betweenness'])

    # D) expand
    frontier = weighted_bfs(anchors, edges_weights={
        'call':1.0, 'type':0.8, 'import':0.6, 'caller':0.9, 'test':0.7
    }, demote_high_fanout=True, max_hops=2)

    # E) fine-grain
    nodes = []
    for n in anchors + frontier:
        if is_large(n):
            nodes.append(static_slice(n, focus_terms=concepts))
        else:
            nodes.append(whole_definition(n))

    # F) budgeted packing
    scored = [(score(n), token_cost(n), n) for n in nodes]
    pack = knapsack_like_greedy(scored, budget_tokens, must_include=anchors)

    # G) package prompt
    return render_prompt(
        task_brief=summarize_task(intent, concepts),
        graph_view=adjacency(pack),
        code_blocks=ordered(pack),
        tests=relevant_tests(pack),
        constraints=derive_constraints(intent, concepts)
    )


⸻

How it works without named symbols (the crux)
	•	Concept→Code bridging happens in steps A–B:
	•	Natural-language concepts from the prompt are expanded to keywords, synonyms, and library/API names.
	•	Those hit comments, docstrings, filenames, commit messages, not just identifiers.
	•	Embeddings pull in semantically similar code even if the names don’t match (e.g., “backoff” vs retry_delay_fn).
	•	API heuristics find framework integration points (HTTP clients, loggers, caches), which are the usual hook sites for changes.
	•	After you have a few anchors (gateway functions/classes/configs), the rest is deterministic graph work to include exactly what’s needed and nothing more.

⸻

Tiny worked example

Prompt: “Add exponential backoff to API retries.”
	•	Concepts: {retry, backoff, http client}.
	•	Queries: “retry policy”, “exponential backoff”, “http client”, “OkHttp interceptor”, “axios retry”, “requests Session”.
	•	Candidates:
	•	http/api_client.py: HttpClient.send() (text + emb)
	•	net/RetryPolicy.kt (keyword “retry”)
	•	config/http.yml (contains retries:)
	•	tests like test_api_retries.py (text)
	•	Anchors: choose HttpClient.send() and RetryPolicy.
	•	Expand: include BackoffStrategy, OkHttpClient builder, config loader, and tests that assert retry behavior.
	•	Slice: if HttpClient.send() is 400 lines, slice only the request/exception/retry loop statements.
	•	Pack: Task brief + anchor defs + BackoffStrategy + one caller + a retry test + config snippet.

⸻

Practical defaults (copy/paste)
	•	Top-K candidates: 60 (cluster to ~12).
	•	Anchors kept: 3–8.
	•	BFS depth: 1 for callees/types, 2 for callers/tests.
	•	Token guardrail: reserve ~65% for anchors + types, ~25% for callers/tests, ~10% for summaries/graph.

⸻

Fallbacks & edge cases
	•	Highly dynamic languages / reflection: lean more on tests and runtime traces (if available) to pick callers.
	•	IoC frameworks (Spring, NestJS): treat annotations/decorators and DI wiring as edges in the graph.
	•	Generated code: de-prioritize by path patterns; prefer the generator or wrapper as the anchor.
	•	No clear anchors found: widen queries, boost embeddings weight, include top 2 modules by semantic similarity and show the LLM both patterns (it can then pinpoint which one to edit).