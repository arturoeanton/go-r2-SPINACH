# Validation evidence

The project distinguishes library conformance, gateway behavior and application behavior. Passing a subset in one layer does not certify the other layers.

## Project tests

Run from the repository root:

```sh
CGO_ENABLED=0 go test ./lab/... ./sparqlite/...
```

The tests verify automatic schema mapping, real BGP joins against SQLite, duplicate projection behavior, a zero LIMIT, comparison with a materialized RDF reference, FILTER/OPTIONAL/COUNT and CONSTRUCT examples, HTTP result encoding, cancellation, explicit rejection of unsupported SERVICE, read-only source access, graph persistence and optimistic revisions. App tests verify that navigation runs for each model action when enabled, disabling it bypasses Jev, timing metadata is opt-in, and client-supplied observations cannot stand in for successful query evidence. Model tests verify the official Jev decision protocol, OpenAI Responses authentication and strict tool definitions, the successful-query gate, actual model identifiers, API error redaction and safe public configuration. Configuration tests cover environment precedence and parsing without shell evaluation. Credential tests cover startup without keys, partial updates that preserve existing keys, persistence across a restart, owner-only file permissions, refusal to follow a credential-file symlink, failed-save rollback, concurrent snapshots and rejection of cross-origin credential writes. API responses are checked for secret leakage.

Frontend tests (`node --test lab/tests/*.test.mjs`) verify exclusion of reading pauses, failed-request accounting, sequential automatic execution, pausing after an in-flight action, resuming, recoverable tool errors, failed requests, escaped context display and timing-off cards.

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

The connection modal now masks key entry, clears secrets on close, saves them on the backend and returns only configuration flags. Startup without a bootstrap environment file and saving from the browser were exercised with disposable test credentials. Stored credentials are owner-only files, not encrypted at rest. Model/provider selectors and the no-AI journey have been removed from the interface. The Jev comparison switch is retained.
