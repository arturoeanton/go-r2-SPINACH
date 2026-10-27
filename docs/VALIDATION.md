# Validation evidence

The project distinguishes library conformance, gateway behavior and application behavior. Passing a subset in one layer does not certify the other layers.

## Local project tests

Run from the repository root:

```sh
CGO_ENABLED=0 go test ./lab/... ./sparqlite/...
```

The tests verify automatic schema mapping, real BGP joins against SQLite, duplicate projection behavior, a zero LIMIT, comparison with a materialized RDF reference, FILTER/OPTIONAL/COUNT and CONSTRUCT examples, HTTP result encoding, cancellation, explicit rejection of unsupported SERVICE, read-only source access, graph persistence and optimistic revisions. App tests verify that Laya is called for each model action and that client-supplied observations cannot stand in for successful query evidence.

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

These are a single warm local run. No p95, sustained concurrency, cold-start, large-graph scaling or comparison against a third-party engine is claimed. Laya's effect on end-to-end SPINACH latency and answer quality has not been benchmarked.

## Application and local-model checks

- The HTTP application served successfully on loopback.
- Browser verification exercised a guided question, its real SPARQL result (Elena Torres / Instituto Aurora), and the English-to-Spanish interface switch.
- LM Studio advertised `google/gemma-4-12b` through `/v1/models`.
- Ollaya advertised Laya variants through `/api/tags` and answered a real navigation request. It selected Research with probability 0.9542 in that probe; that value is a model option score, not a correctness guarantee.
- An initial live Gemma probe produced a search action and a subsequent generation timed out. Later probes attempted premature answers, which the backend rejected. The structured-output action schema now excludes `answer` until a successful query exists, and replay preserves that gate.
- A final live journey completed successfully in two model actions: Laya selected Collaboration (initial probability 0.9986), Gemma executed a query that returned Sofía Vega and Lina Costa, and its English answer named exactly those two collaborators. The navigation decision was re-evaluated before each action. See [live-model-check.json](live-model-check.json). This is a functional check, not a reliability or latency benchmark.
- Browser verification also uploaded the disposable business SQLite fixture, ran a three-pattern query with three correct result rows via SQL pushdown, created and saved an editor node, then used undo and save to restore the original example. The browser recorded no console errors in that check.
- Automated application tests use controlled model responses to verify orchestration independently of model latency and quality.

## Release gates

1. Resolve the known evaluator failures and federated-query gap.
2. Validate the gateway, native store, result formats and HTTP dataset/update protocols, not only the upstream evaluator.
3. Specify or explicitly exclude inverse relational updates; arbitrary SQLite schemas do not make that mapping unambiguous.
4. Broaden SQLite fixtures: mixed-type keys, collations, nullable composite primary keys, WITHOUT ROWID, views, generated columns and malformed foreign-key declarations.
5. Measure warm/cold latency, concurrency and representative large datasets.
6. Compare navigation with/without Laya on the same questions, measuring both latency and answer quality.

Until these gates pass, this is an experimental implementation rather than a 100%-conformant high-performance product claim.
