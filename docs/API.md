# HTTP API reference

[English README](../README.md) · [Spanish README](../README.es.md) · [Validation](VALIDATION.md)

Reviewed against the repository on **2026-10-27**. The lab's JSON API and the standalone SPARQL endpoint are different interfaces. Examples use the supplied local binaries; a public deployment may run a different revision.

## 1. Lab JSON API — port 8090

Start from the repository root:

```sh
go run ./lab/cmd/spinach -db ./lab/data/spinach.db
```

All API paths below start with `/api`. Non-GET/HEAD requests require `Content-Type: application/json`, including DELETE. JSON-decoded bodies reject unknown fields and extra JSON values; the request body ceiling is 24 MiB. Errors normally use `{"error":"message"}`. A journey provider failure can additionally include `trace` and, when requested, `timing`.

This edition has no application authentication or per-user data ownership. The binary binds only to loopback; origin checks are not a substitute for authentication on a hosted instance. The browser never supplies provider API keys or model IDs. The server loads those once at startup from environment variables and/or `.env`.

### Route inventory

| Method and path | Request | Response / behavior |
| --- | --- | --- |
| `GET /graphs` | — | Array of saved graph documents. |
| `POST /graphs` | Complete graph document; ID and revision are assigned by the server. | `201` graph with revision `1`. |
| `GET /graphs/{graphId}` | — | One graph, or `404`. |
| `PUT /graphs/{graphId}` | Complete graph document with its current `revision`. | Updated graph; stale revision returns `409`. |
| `DELETE /graphs/{graphId}` | JSON content type; `{}` is sufficient. | `{"deleted":true,"sourceDeleted":true|false}`; also cleans up an uploaded SQLite copy when the last graph referencing it is removed. |
| `POST /graphs/{graphId}/query` | `{"query":"SELECT ..."}` | Lab result envelope, described below. No model call. |
| `POST /graphs/{graphId}/route` | Journey request. | Initial navigation decision, or a disabled/structural decision. |
| `POST /graphs/{graphId}/step` | Journey request and previous events. | One executed model action with observation and trace. |
| `POST /import` | `{"format":"json|nt|ttl|rdfxml","title":"...","content":"..."}` | `201` imported graph. `content` is text, including a stringified graph for JSON import. |
| `POST /databases` | `{"title":"...","content":"BASE64_SQLITE_BYTES"}` | `201` graph containing `sqliteSource`; the decoded file limit is 16 MiB. |
| `GET /databases/{sourceId}/schema` | — | Inspected tables, columns, identities, foreign keys, indexes and mapping warnings. |
| `GET /examples` | — | Built-in fictional seed graph documents. |
| `GET /config` | — | Public provider labels and configured model IDs; no keys. |
| `GET /status` | — | Public config plus `generation` and `navigation` service status, URLs and available model catalogs. |

A graph ID and a SQLite source ID are separate identifiers. Use the graph ID to query or delete the imported universe, and `sqliteSource` to inspect its database schema. There is no separate `DELETE /databases/{sourceId}` route: deletion is driven by the graph reference so shared sources can be preserved.

### Graph documents

An editable graph has this shape:

```json
{
  "id": "assigned-by-server",
  "title": "My universe",
  "description": "Example data",
  "nodes": [
    {"id":"https://example.org/alice","label":"Alice","kind":"iri","group":"People","x":200,"y":200}
  ],
  "edges": [],
  "subgraphs": [],
  "examples": [],
  "revision": 1
}
```

Node kinds are `iri`, `literal` and `bnode`. Literal values are carried in `label`, with optional `datatype` or `language`. Edges have `id`, `source`, `target`, `predicate` (an absolute IRI) and `label`. Subgraphs have `id`, `name`, `description` and a `nodes` array of existing node IDs. Examples have `question` and `query`.

Limits include 400 nodes, 2,000 edges, 24 subgraphs, 30 examples, a 160-byte title and a 4,000-byte description. Further validation covers identifiers, IRI syntax, coordinates, node references and field lengths; see `lab/internal/domain/graph.go`.

`sqliteSource` identifies an uploaded read-only database. Layout/subgraph edits are supported in the UI; editing the visual preview does not update the underlying SQL tables. Importing an exported JSON document that refers to a missing source returns `422`; the JSON does not contain the SQLite file.

### Manual SPARQL query

```sh
curl http://127.0.0.1:8090/api/graphs/aurora/query \
  -H 'Content-Type: application/json' \
  --data-binary '{"query":"SELECT ?person WHERE { ?person ex:investiga ex:fisica . }"}'
```

The result contains `variables`, `rows`, optional `boolean`, `edgeIds`, `nodeIds`, `truncated`, `engine` and `durationMs`. Each bound row cell includes `value`, `label` and `kind`, and may include `datatype`, `language` and `nodeId`. A SQL-pushdown result also includes `sql` and `plan` when available. Editable graphs recognize implicit `ex:`, `rdfs:` and `xsd:` prefixes. For SQLite mappings, inspect the generated vocabulary and use its IRIs.

The lab displays up to 500 rows, with `truncated: true` when more exist. The graph preview of an uploaded SQLite database is at most 80 triples, while its SPARQL queries address the full uploaded source. The search/explore actions inspect the graph document, so on a SQLite import they inspect that preview rather than every database row.

### Free-form question journeys

The request contract is:

```json
{
  "question": "Who studies physics and works in Paris?",
  "language": "en",
  "useNavigation": true,
  "measureTime": true,
  "events": []
}
```

- `question` is required, nonblank, and at most 2,000 UTF-8 bytes; it need not match an example.
- `language` selects `es` or defaults to `en` for the generated objective and answer.
- `useNavigation` defaults to `true`; `false` bypasses Jev calls.
- `measureTime` defaults to `false` in the API. The UI enables it by default and remembers the choice.
- `events` contains previous returned events in chronological order. `/step` rejects a ninth action; at most eight actions may execute per journey.
- The legacy `focus` field is accepted but is not used to choose `/step` context. The server obtains focus from Jev when enabled and uses an unfiltered preview when disabled.
- Provider/model override fields are rejected. There is no endpoint for writing model settings or credentials.

Call `/route` for an initial decision if you want to reproduce the UI's initial-selection card, then call `/step` once per action. `/step` performs its own navigation decision before each action; `/route` is not a prerequisite. The frontend implements **Run all steps** by making sequential `/step` calls rather than using a separate run endpoint.

A step returns:

```json
{
  "action": {"type":"query","objective":"Find matching people","query":"SELECT ..."},
  "observation": {"variables":[],"rows":[],"edgeIds":[],"nodeIds":[],"truncated":false,"engine":"...","durationMs":0},
  "generation": {"provider":"openai","model":"actual-provider-model-id","input":{},"instructions":"...","usage":{}},
  "timing": {"totalMs":0,"navigationMs":0,"generationMs":0,"replayMs":0,"toolMs":0}
}
```

This is an illustrative shape, not a recorded response. `navigation` is present when enabled; `timing` is present when requested; `generation.usage` appears only if the provider returned usage. The navigation response uses `choice`, `candidates`, `confidence`, `model`, `source` and optional input/timing. Its numeric scores rank subgraph options and are not answer-confidence estimates.

Actions are `search` (`text`), `explore` (`nodeId`), `query` (`query`) or `answer` (`answer`). `observation` depends on the action. A tool failure can return HTTP `200` with an event `error`; append that event so the next action can correct the failure. Provider/request failures return a non-2xx response and stop automatic execution. Stop on a successful `answer` or the action limit. Pausing means not sending the next request; cancellation uses the current request's connection/context.

The server reconstructs previous observations by executing the submitted actions against current data; it does not trust client-provided evidence. An answer requires a successful query, including a legitimate empty result. This gate does not prove that the query correctly captures the question. The generator receives no example reference queries, and it has no Internet/Wikidata tool.

### SQLite removal and failure handling

The UI asks for confirmation before calling `DELETE /graphs/{graphId}`. Removing the last reference closes the cached source connection and removes the app-owned file; the user's original file is unchanged. With other graph references, `sourceDeleted` is `false` and their queries continue to work. A repeated graph deletion returns `404`. Invalid source IDs are rejected rather than interpreted as paths.

The handler serializes graph writes and cleanup. If file removal fails, it attempts to restore the graph so the user can reload and retry; failure of that restoration is logged. The database and filesystem do not form a crash-atomic transaction. Failed imports clean up their newly uploaded copies when later preview or graph validation fails.

Typical error statuses: `400` malformed/unknown JSON fields or missing update revision; `403` rejected origin; `404` missing graph/source; `409` stale graph revision; `415` wrong content type; `422` invalid graph/question/import or query failure; `500` storage failure; `502` model-provider failure. Check both HTTP status and a returned journey event's `error`.

## 2. Standalone SPARQLite — port 8091

```sh
go run ./sparqlite/cmd/make-example -out examples/business.sqlite
go run ./sparqlite/cmd/sparqlite -db examples/business.sqlite
```

The example generator creates its parent directory and refuses to overwrite an existing file. `GET /schema` returns the automatic vocabulary. `/sparql` accepts:

- GET with a `query` parameter.
- POST with `application/sparql-query` and a SPARQL text body.
- POST with `application/x-www-form-urlencoded` and a `query` field.

The POST body limit is 1 MiB, and the source query-text limit is 100,000 bytes. Queries have a 20-second HTTP timeout, a 200,000-intermediate-solution evaluator budget, and a 100,000-row SQL-pushdown guard. Limit failures are errors rather than silent successful truncation.

```sh
curl http://127.0.0.1:8091/sparql \
  -H 'Content-Type: application/sparql-query' \
  -H 'Accept: application/sparql-results+json' \
  --data-binary 'SELECT ?name WHERE {
    ?person <https://sqlite.local/customers#name> ?name .
  }'
```

SELECT/ASK support negotiated JSON, XML, CSV or TSV. Graph results are returned as Turtle. Headers include `X-SPARQLite-Engine` (`sql-pushdown` or `sparql-evaluator`) and `Server-Timing`. These responses use standard SPARQL result formats, not the lab's JSON result envelope. Errors are plain text.

Relational updates are rejected; neither SPARQL Update nor a visual graph change writes back to the original tables. Use query `FROM`/`FROM NAMED` clauses for the mapped table graphs. The HTTP `default-graph-uri` and `named-graph-uri` parameters return `422`. `SERVICE` is unsupported.

## 3. Native RDF mode is separate

```sh
go run ./sparqlite/cmd/sparqlite -rdf-store examples/knowledge.sqlite -write
```

This mode delegates `/sparql` and `/data` to goRDFlib's query/update and Graph Store handlers. Without `-write`, it is read-only. `/schema` belongs to relational mapping mode and is not registered in native RDF mode. Remote `LOAD` is disabled by the dependency defaults. Multi-operation updates are not promised to be atomic, and this mode does not inherit every correction made by the lab or relational wrapper. See the [validation report](VALIDATION.md) before relying on any conformance claim.
