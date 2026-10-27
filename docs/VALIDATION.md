# Validation evidence

[English README](../README.md) · [Spanish README](../README.es.md) · [HTTP API](API.md)

Documentation reviewed on **2026-10-27**. Recorded API timestamps retain their original UTC values; current configuration and historical test results are distinguished below.

The project distinguishes library conformance, gateway behavior and application behavior. Passing a subset in one layer does not certify the other layers.

## Current configuration and evidence at a glance

The local app was returned to `gpt-6.1-sol` with `medium` effort after the Luna experiment. `.env.example` now selects the same Sol model; the unconfigured constructor fallback remains `gpt-5`. The public demo's last configuration check also reported `gpt-6.1-sol`, but its hosted revision and end-to-end behavior must be verified separately. This documentation refresh does not deploy or rerun paid inference.

| Layer / run | Recorded result | What it establishes |
| --- | --- | --- |
| Repository regression suite | Go modules passed; 7 Node tests passed after the SQLite removal change. | Tested application, adapter, storage, gateway and frontend contracts. |
| W3C dependency audit | 513 passed, 9 failed. | The selected upstream test functions only; not full product conformance. |
| GPT-5 complex-question matrix | 12/12 expected final answers and evidence; 5 intermediate parser failures recovered. | Historical functional validation with and without Jev. |
| Luna complex-question matrix | 12/12 completed; 11/12 semantic passes; 10/12 language passes; 9/12 passed both. | Faster observed run with documented correctness failures. |
| Sol | Active configuration and service connectivity checked. | No equivalent 12-case Sol evaluation is recorded. |

## Project tests

Run from the repository root:

```sh
CGO_ENABLED=0 go test ./lab/... ./sparqlite/...
```

The tests verify automatic schema mapping, real BGP joins against SQLite, duplicate projection behavior, a zero LIMIT, comparison with a materialized RDF reference, FILTER/OPTIONAL/COUNT and CONSTRUCT examples, HTTP result encoding, cancellation, explicit rejection of unsupported SERVICE, read-only source access, graph persistence and optimistic revisions. App tests verify that navigation runs for each model action when enabled, disabling it bypasses Jev, timing metadata is opt-in, and client-supplied observations cannot stand in for successful query evidence. Model tests verify the official Jev decision protocol, OpenAI Responses authentication and strict tool definitions, the successful-query gate, actual model identifiers, API error redaction and safe public configuration. Configuration tests cover environment precedence and parsing without shell evaluation. Configuration tests also cover startup without keys, server-selected model IDs for routing and generation, rejection of browser model overrides, unavailable HTTP configuration writes and immutable startup configuration. API responses are checked for secret leakage.

Frontend tests (`node --test lab/tests/*.test.mjs`) verify exclusion of reading pauses, failed-request accounting, sequential automatic execution, pausing after an in-flight action, resuming, recoverable tool errors, failed requests, escaped context display, timing-off cards and the SQLite confirmation/cancellation/failure workflow. Storage and HTTP tests exercise actual uploaded-file deletion and shared-source preservation; see the dedicated section below.

These tests are representative regression tests, not an exhaustive SPARQL specification suite.

## W3C dependency audit

- Evaluator: `github.com/tggo/goRDFlib v0.9.3`.
- Suite: [w3c/rdf-tests](https://github.com/w3c/rdf-tests), commit `bfefa770267ec0e5b6e45764bbca4ee43789ba72`.
- Engine unchanged; the suite was provided in its expected `testdata/w3c/rdf-tests` location.
- Command: `go test -json github.com/tggo/goRDFlib/sparql -run 'TestW3C($|Dataset|Update)' -count=1` in the gateway module context.
- Results count leaf test cases, excluding aggregate parent tests.

| Suite section | Passed | Failed |
| --- | ---: | ---: |
| SPARQL 1.1 query and syntax | 344 | 9 |
| Dataset clauses | 12 | 0 |
| SPARQL 1.1 Update and syntax | 157 | 0 |
| **Total executed** | **513** | **9** |

The exact case list is in [conformance-results.json](conformance-results.json). The command exits unsuccessfully, as expected when conformance failures exist. No skipped cases were reported among the selected test functions. Other specifications, protocol functions, manifest entry types and integrations were not established by this run.

The failures were:

1. VALUES inside GRAPH binding the graph name variable, with FROM NAMED.
2. Graph name variable bound outside GRAPH to graphs outside the dataset, with FROM NAMED.
3. GRAPH with a fixed IRI outside the dataset around VALUES, with FROM NAMED.
4. Post-subquery VALUES with OFFSET and LIMIT.
5. Post-query VALUES with GROUP BY and projected VALUES variable.
6. Post-query VALUES with GROUP BY and VALUES variable in SELECT expression.
7. Positive syntax: GROUP BY with trailing VALUES variable in projection.
8. Positive syntax: GROUP BY with trailing VALUES variable in SELECT expression.
9. Negative syntax: GROUP BY with trailing VALUES variable reassigned by SELECT expression.

Separate inspection and a regression test confirmed that the pinned parser rejects federated SERVICE. A separate local differential test also exposed the evaluator's handling of LIMIT 0. The gateway and educational query wrapper now return an empty result for that case; the dependency itself was not modified. The native RDF endpoint still delegates to the unmodified dependency and does not inherit every wrapper correction.

Run `sh scripts/check-conformance.sh` to repeat the audit using a scratch copy of the dependency and the pinned W3C data. This downloads public source/test data, leaves the dependency cache unchanged and preserves the nonzero test exit status.

## Performance probe

Command:

```sh
go test ./sparqlite -run '^$' -bench BenchmarkMapping -benchmem -benchtime=1s
```

Measured on an Apple M4, macOS arm64, Go 1.27.1, CGO disabled:

| Probe | Time per operation | Allocations |
| --- | ---: | ---: |
| Exact name match over 10,003 customer rows | 517,791 ns (~0.52 ms) | 314 |
| Three-pattern foreign-key join | 107,810 ns (~0.108 ms) | 608 |

The fixture has 10,003 customer rows and three order rows. The second probe is small and selective. The first has no name index. Fixture creation is outside the timed section; parser, cached-schema lookup, transaction, compilation, EXPLAIN and execution are inside it. See [raw benchmark output](benchmark-results.txt).

These are a single warm local run. No p95, sustained concurrency, cold-start, large-graph scaling or comparison against a third-party engine is claimed. Jev's effect on end-to-end SPINACH latency and answer quality has not been benchmarked.

## Application and real API checks

The app uses Jev through TypeSafe and OpenAI Responses exclusively. A real authenticated check on 2026-10-27 UTC used the fictional Aurora graph and the question “Who does Elena collaborate with?”. Both service catalogs were reachable. Configuration requested `jev-latest` and `gpt-5`; OpenAI returned `gpt-5-2025-08-07`.

Two journeys completed:

| Mode | Model actions | Query evidence | Final answer |
| --- | --- | --- | --- |
| Jev enabled | query → answer | Sofía Vega and Lina Costa | Both collaborators correctly named |
| Jev disabled | query → answer | Sofía Vega and Lina Costa | Both collaborators correctly named |

Jev chose Collaboration initially and was called again before each action when enabled. Without Jev, navigation was absent and the reported navigation duration was exactly zero. Each answer followed a real successful SPARQL query. [Recorded requests, results and model traces](live-model-check.json) contain only the fictional test graph and API outputs, without credentials.

In that single run, the two server step totals were approximately 8.52 s and 5.36 s with Jev; without Jev they were 5.62 s and 3.77 s. These are functional observations, not a controlled comparison: request ordering, provider latency and model variability are uncontrolled. No speedup or reliability estimate follows from them.

Earlier browser checks verified real SPARQL results, English/Spanish switching, SQLite upload and SQL pushdown, and editor save/undo. Automated application tests use controlled model responses to check orchestration independently of live latency and quality.

## Journey controls and trace verification

Question journeys always use OpenAI with optional Jev navigation. They can run one action at a time or automatically, with pause after the in-flight action and cancellation that retains completed cards. Each card records the responsible provider/model, observable input, action, output and duration. Timing can be hidden. Browser processing time excludes reading pauses; server timing separates navigation, generation, evidence replay and tools. Journey traces remain in the current page session.

Traces expose application inputs, actions and observations; they do not expose private model reasoning. Frontend tests cover these controls and trace rendering. See the API record above for real Jev and GPT-5 outputs.

## Release gates

1. Resolve the known evaluator failures and federated-query gap.
2. Validate the gateway, native store, result formats and HTTP dataset/update protocols, not only the upstream evaluator.
3. Specify or explicitly exclude inverse relational updates; arbitrary SQLite schemas do not make that mapping unambiguous.
4. Broaden SQLite fixtures: mixed-type keys, collations, nullable composite primary keys, WITHOUT ROWID, views, generated columns and malformed foreign-key declarations.
5. Measure warm/cold latency, concurrency and representative large datasets.
6. Compare navigation with/without Jev on the same questions, measuring both latency and answer quality.

Until these gates pass, this is an experimental implementation rather than a 100%-conformant high-performance product claim.

## Complex-question fixture

The Research network fixture contains 99 nodes, 295 edges and six subgraphs. Six reference questions are tested against exact expected evidence: multihop country/project/funder joins (4 rows), distinct-country aggregation (8 projects), a negative retraction condition (1 researcher), transitive partnerships (2 institutions), a deduplicated citation aggregate (Project 05, 100 citations), and cross-country collaboration/project/coauthor joins (8 researcher pairs). The generator no longer receives example SPARQL queries. The earlier simple API record predates this removal and is functional evidence only.

## Final complex-question API matrix

The [final validation matrix](final-validation-matrix.json), recorded on 2026-10-27 UTC, contains all six complex questions with Jev enabled and disabled: **12 of 12 journeys produced the expected query evidence and correct final answers**. Configuration used `gpt-5`, `jev-latest` and `medium` reasoning effort. Evidence comparison accepts equivalent projections and treats collaborator pairs as unordered; final lists, exclusions and measures were also reviewed. The negative-condition regression adds a researcher with a retracted paper to the eligible project and verifies that the exclusion still removes that researcher.

The generator receives actual typed RDF literal values and one observed subject-to-object example per predicate (up to 128 predicates), plus bounded previews and entity metadata. It receives no reference queries. An earlier failed citation run exposed missing numeric literals in the preview; the final matrix follows that fix and the updated instructions for relation direction, deduplication and independent conditions.

Three Jev-enabled journeys recovered from parser errors: question 1 corrected one failed query, question 5 corrected one, and question 6 corrected three. These failures are visible in the traces before the successful evidence. A known parser limitation includes rejecting an optional dot after a braced UNION group. A successful final answer does not mean every intermediate query succeeded.

Two independent journeys ran concurrently to reduce validation time. This is functional validation, **not a latency benchmark or evidence of a Jev speedup**. The isolated citation record in `complex-model-check.json` used an earlier reasoning setting; `complex-model-check.before-fix.json` is preserved historical failure evidence. Neither replaces the final matrix.

## Server-only configuration

API keys, model IDs, API base URLs and reasoning effort are loaded once from process environment variables and/or `.env`; process variables take precedence. Changes require a restart. The browser has a read-only service indicator and model badges, with no key form or model selector. HTTP credential updates and client model overrides are unavailable. The former file-backed credential store and its configuration endpoint have been removed; the application no longer reads `model-credentials.json`. No saved credential file existed in the main local installation when this change was made.

The Jev comparison switch is retained. The no-AI journey remains removed. Startup without keys still permits editing and manual queries; inference reports a missing server connection rather than substituting a service.

After the server-only configuration change, a fresh browser journey on Aurora completed with real Jev and GPT-5 calls: query → answer, returning Sofía Vega and Lina Costa with two matching evidence rows. The running HTTP server rejected both client model override fields with 400 and rejected configuration writes with 405. Browser inspection found no password inputs, configuration buttons or model selectors; the read-only connection indicator was verified in English and Spanish.

## GPT-6 Luna local evaluation

The [Luna matrix](luna-validation-matrix.json) repeats the same six questions with and without Jev using `gpt-6-luna` at explicit `medium` effort. The prompts, graph, eight-action limit and 6,000-token output budget are unchanged from the GPT-5 baseline. Luna was selected only for this local experiment. The local `.env` was subsequently returned to Sol (`gpt-6.1-sol`); the source fallback remains GPT-5. The Vercel demo was checked separately and reported `gpt-6.1-sol`. These configuration observations are not a Sol correctness benchmark.

All 12 journeys finished and their final answers matched the original fixture. However, a semantic review passed only **11 of 12**: question 3 with Jev reversed `authored` inside `NOT EXISTS`, so its correct-looking answer was accidental. The [adversarial replay](luna-adversarial-check.json) adds Zoe Flores, an author with a retracted paper, as an otherwise eligible leader. The reference query and the Luna-without-Jev query exclude her; the Luna-with-Jev query incorrectly includes her. Two answers also ignored the requested English language and used Spanish. Overall, **9 of 12** passed both semantic and language checks.

There were zero parser errors, but two journeys needed a second query after a semantic mistake returned no rows. Both citation queries returned a full descending ranking; the highest row and final answer are correctly Project 05 with 100 citations. Their initial automatic row-count comparison failed because the reference returns just one row; that raw result is preserved alongside the manual review.

The observed median active journey time was **5.95 seconds**, versus **47.69 seconds** in the earlier GPT-5 matrix. These are separate functional runs with two concurrent journeys, not a controlled or repeated speed benchmark. They do not prove equivalent quality or a general speedup from Luna or Jev.

The 26 OpenAI calls reported 262,135 input tokens (258,859 cache-write tokens; no cache hits) and 5,111 output tokens, including reasoning. Using the historical pricing assumptions recorded in the matrix from the [Luna model page](https://developers.openai.com/api/docs/models/gpt-6-luna), including 30-minute cache-write pricing, the estimate is **US$0.03524** for these 12 journeys. This excludes Jev and is an estimate rather than a billing statement. Generation traces retain provider usage, including on incomplete responses. This is a historical cost estimate; it is not a current pricing quote or a guarantee for future runs.


## SQLite upload removal — 2026-10-27

- Storage tests use a temporary real SQLite database and verify deletion of the uploaded copy, closure of cached connections, preservation of the original, rejection of invalid IDs, repeat cleanup, and reimport.
- HTTP tests cover import → query → delete, inaccessible schema after deletion, independent uploads and seed preservation, shared-source reference cleanup, retry after a simulated removal failure, rejection of stale references, and cleanup after a failed import.
- Frontend tests cover no deletion before confirmation, stopping an active journey before deletion, retaining the dialog's target, and keeping unsaved changes after a failure.
- Browser inspection verified the English and Spanish confirmations and cancellation on the local app. A query after cancellation returned the expected three rows. The existing user-uploaded database was preserved.


## Evidence inventory and interpretation

| Artifact | Purpose |
| --- | --- |
| [conformance-results.json](conformance-results.json) | Selected W3C leaf-case outcomes and pinned versions. |
| [benchmark-results.txt](benchmark-results.txt) | Original local gateway microbenchmark output. |
| [live-model-check.json](live-model-check.json) | Early Aurora API journeys; predates removal of example-query context. |
| [complex-model-check.before-fix.json](complex-model-check.before-fix.json) | Preserved failed citation run before context corrections. |
| [complex-model-check.json](complex-model-check.json) | Isolated citation check at an earlier reasoning setting. |
| [final-validation-matrix.json](final-validation-matrix.json) | Historical GPT-5 matrix for all six complex questions in both navigation modes. |
| [luna-validation-matrix.json](luna-validation-matrix.json) | Luna runs, raw grader outcomes, semantic review, language checks, usage and historical cost assumptions. |
| [luna-adversarial-check.json](luna-adversarial-check.json) | Counterexample exposing the reversed exclusion relation. |

Raw records are preserved rather than rewritten to match the active configuration. A correct-looking answer on one fixture is insufficient: the Luna counterexample demonstrates why relation direction and exclusion semantics require review. The Jev speed hypothesis remains open. Evaluate repeated, interleaved runs on unchanged data with the same generator and effort, compare correct answers and query semantics as well as elapsed time, and report retries, failures and usage. Do not turn the earlier separate GPT-5/Luna timings into a causal claim about Jev or a Sol quality estimate.
