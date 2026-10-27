package sparqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tggo/goRDFlib/graph"
	"github.com/tggo/goRDFlib/sparql"
	"github.com/tggo/goRDFlib/term"
)

func fixture(t testing.TB, n int) *Source {
	t.Helper()
	path := filepath.Join(t.TempDir(), "business.sqlite")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`PRAGMA foreign_keys=ON; CREATE TABLE customers(id INTEGER PRIMARY KEY,name TEXT NOT NULL, city TEXT);
 CREATE TABLE orders(id INTEGER PRIMARY KEY,customer_id INTEGER REFERENCES customers(id),amount REAL);
 CREATE INDEX orders_customer ON orders(customer_id);
 CREATE TABLE "odd' table"("value""name" TEXT);
 INSERT INTO customers VALUES(1,'Ana','París'),(2,'Luis',NULL),(3,'Ana','Roma');
 INSERT INTO orders VALUES(10,1,20.5),(11,1,12),(12,3,99);
 INSERT INTO "odd' table" VALUES('escaped');`)
	// SQLite quotes use doubled quotes, not backslashes.
	if e != nil {
		db.Close()
		t.Fatal(e)
	}
	if n > 0 {
		tx, e := db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		stmt, e := tx.Prepare("INSERT INTO customers(id,name,city) VALUES(?,?,?)")
		if e != nil {
			t.Fatal(e)
		}
		for i := 4; i < n+4; i++ {
			if _, e = stmt.Exec(i, "Persona", "Ciudad"); e != nil {
				t.Fatal(e)
			}
		}
		stmt.Close()
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	db.Close()
	s, e := Open(path, "")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func signatures(r *sparql.Result) []string {
	if r.Type == "ASK" {
		if r.AskResult {
			return []string{"true"}
		}
		return []string{"false"}
	}
	out := []string{}
	for _, row := range r.Bindings {
		keys := []string{}
		for k := range row {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := []string{}
		for _, k := range keys {
			parts = append(parts, k+"="+term.TermKey(row[k]))
		}
		out = append(out, strings.Join(parts, "|"))
	}
	sort.Strings(out)
	return out
}
func TestAutomaticMappingAndSQLAgreeWithReference(t *testing.T) {
	s := fixture(t, 0)
	ctx := context.Background()
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		t.Fatal(e)
	}
	schema, e := s.schema(ctx, tx)
	if e != nil {
		t.Fatal(e)
	}
	vs := &virtualStore{db: tx, relations: relations(schema), base: s.Base}
	memory := graph.NewGraph()
	for tr, e := range vs.Triples(ctx, term.TriplePattern{}, nil) {
		if e != nil {
			t.Fatal(e)
		}
		if e = memory.Add(ctx, tr.Subject, tr.Predicate, tr.Object); e != nil {
			t.Fatal(e)
		}
	}
	tx.Rollback()
	queries := []string{
		`SELECT ?s ?name WHERE { ?s <https://sqlite.local/customers#name> ?name }`,
		`SELECT ?name WHERE { ?s <https://sqlite.local/customers#name> ?name }`,
		`SELECT DISTINCT ?name WHERE { ?s <https://sqlite.local/customers#name> ?name }`,
		`SELECT ?name ?amount WHERE { ?o <https://sqlite.local/orders#ref-customer_id> ?c . ?c <https://sqlite.local/customers#name> ?name . ?o <https://sqlite.local/orders#amount> ?amount }`,
		`ASK { ?s <https://sqlite.local/customers#name> "Ana" }`,
		`ASK { ?s <https://sqlite.local/customers#name> "Nadie" }`,
		`SELECT ?s WHERE { ?s <https://sqlite.local/customers#name> "Ana" } LIMIT 0`,
		`SELECT ?s ?city WHERE { ?s <https://sqlite.local/customers#name> "Ana" OPTIONAL { ?s <https://sqlite.local/customers#city> ?city } } ORDER BY ?s`,
		`SELECT (COUNT(?s) AS ?n) WHERE { ?s <https://sqlite.local/customers#name> ?name }`,
		`SELECT ?s WHERE { ?s <https://sqlite.local/orders#amount> ?amount FILTER(?amount > 20) }`,
		`CONSTRUCT { ?s <urn:name> ?n } WHERE { ?s <https://sqlite.local/customers#name> ?n }`,
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			got, report, e := s.Query(ctx, q)
			if e != nil {
				t.Fatal(e)
			}
			want, e := sparql.QueryContext(ctx, memory, q)
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(q, "LIMIT 0") {
				if len(got.Bindings) != 0 {
					t.Fatal("LIMIT 0 returned rows")
				}
				return
			}
			if got.Graph != nil {
				gn, _ := got.Graph.Len(ctx)
				wn, _ := want.Graph.Len(ctx)
				if gn != wn {
					t.Fatalf("construct %d != %d", gn, wn)
				}
				return
			}
			if !reflect.DeepEqual(signatures(got), signatures(want)) {
				t.Fatalf("%s\ngot %v\nwant %v", report.Engine, signatures(got), signatures(want))
			}
		})
	}
}
func TestSchemaAndReadOnly(t *testing.T) {
	s := fixture(t, 0)
	schema, e := s.Inspect(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(schema.Tables) != 3 {
		t.Fatalf("tables %d", len(schema.Tables))
	}
	if _, e = s.db.Exec("DELETE FROM customers"); e == nil {
		t.Fatal("source writable")
	}
	var n int
	if e = s.db.QueryRow("SELECT count(*) FROM customers").Scan(&n); e != nil || n != 3 {
		t.Fatalf("source changed: %d %v", n, e)
	}
}
func TestHTTPProtocol(t *testing.T) {
	s := fixture(t, 0)
	handler := s.Handler()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/sparql", strings.NewReader(`SELECT ?s WHERE { ?s <https://sqlite.local/customers#name> "Ana" }`))
	req.Header.Set("Content-Type", "application/sparql-query")
	handler.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	var got map[string]any
	if e := json.NewDecoder(res.Body).Decode(&got); e != nil {
		t.Fatal(e)
	}
	if res.Header.Get("X-SPARQLite-Engine") != "sql-pushdown" {
		t.Fatal("not SQL")
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/sparql", strings.NewReader("CLEAR ALL"))
	req.Header.Set("Content-Type", "application/sparql-update")
	handler.ServeHTTP(w, req)
	r := w.Result()
	defer r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal(r.Status)
	}
}
func TestCancellationAndUnsupportedService(t *testing.T) {
	s := fixture(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e := s.Query(ctx, `SELECT * WHERE {?s ?p ?o}`); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, _, e := s.Query(context.Background(), `SELECT * WHERE { SERVICE <http://127.0.0.1:1/> {?s ?p ?o} }`); e == nil {
		t.Fatal("unsupported SERVICE silently ignored")
	}
}
func BenchmarkMappingQuery10K(b *testing.B) {
	s := fixture(b, 10000)
	q := `SELECT ?s WHERE { ?s <https://sqlite.local/customers#name> "Ana" }`
	b.ResetTimer()
	for b.Loop() {
		if _, _, e := s.Query(context.Background(), q); e != nil {
			b.Fatal(e)
		}
	}
}
func BenchmarkMappingJoin10K(b *testing.B) {
	s := fixture(b, 10000)
	q := `SELECT ?name ?amount WHERE { ?o <https://sqlite.local/orders#ref-customer_id> ?c . ?c <https://sqlite.local/customers#name> ?name . ?o <https://sqlite.local/orders#amount> ?amount }`
	b.ResetTimer()
	for b.Loop() {
		if _, _, e := s.Query(context.Background(), q); e != nil {
			b.Fatal(e)
		}
	}
}
