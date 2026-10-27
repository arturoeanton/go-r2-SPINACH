package sparqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tggo/goRDFlib/graph"
	"github.com/tggo/goRDFlib/nt"
	"github.com/tggo/goRDFlib/sparql"
	"github.com/tggo/goRDFlib/store"
	"github.com/tggo/goRDFlib/term"
)

type Report struct {
	Engine     string   `json:"engine"`
	DurationMS float64  `json:"durationMs"`
	SQL        string   `json:"sql,omitempty"`
	Parameters []any    `json:"parameters,omitempty"`
	Plan       []string `json:"plan,omitempty"`
}

func urlPart(s string) string { return url.PathEscape(s) }

// Query uses one read transaction for schema discovery and all evaluation reads.
// Basic graph patterns are compiled to one SQL query; other algebra is evaluated
// by goRDFlib over a lazy SQL-backed Store, without copying the database to RDF.
func (s *Source) Query(ctx context.Context, text string) (*sparql.Result, Report, error) {
	return s.query(ctx, text, true)
}
func (s *Source) QueryEvaluated(ctx context.Context, text string) (*sparql.Result, Report, error) {
	return s.query(ctx, text, false)
}
func (s *Source) query(ctx context.Context, text string, pushdown bool) (*sparql.Result, Report, error) {
	started := time.Now()
	report := Report{Engine: "sparql-evaluator", Plan: []string{}}
	if len(text) > 100000 {
		return nil, report, fmt.Errorf("consulta demasiado extensa")
	}
	q, e := sparql.Parse(text)
	if e != nil {
		return nil, report, e
	}
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, report, e
	}
	defer tx.Rollback()
	schema, e := s.schema(ctx, tx)
	if e != nil {
		return nil, report, e
	}
	rels := relations(schema)
	if pushdown {
		if sqlText, args, vars, ok := compile(q, rels); ok {
			report.Engine = "sql-pushdown"
			report.SQL = sqlText
			report.Parameters = args
			rows, err := tx.QueryContext(ctx, "EXPLAIN QUERY PLAN "+sqlText, args...)
			if err != nil {
				return nil, report, err
			}
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
					rows.Close()
					return nil, report, err
				}
				report.Plan = append(report.Plan, detail)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, report, err
			}
			result := &sparql.Result{Type: q.Type, Vars: vars, Bindings: []map[string]term.Term{}}
			rows, err = tx.QueryContext(ctx, sqlText, args...)
			if err != nil {
				return nil, report, err
			}
			defer rows.Close()
			for rows.Next() {
				if q.Type == "ASK" {
					result.AskResult = true
					break
				}
				keys := make([]string, len(vars))
				dest := make([]any, len(vars))
				for i := range keys {
					dest[i] = &keys[i]
				}
				if err = rows.Scan(dest...); err != nil {
					return nil, report, err
				}
				row := map[string]term.Term{}
				for i, key := range keys {
					t, err := term.TermFromKey(key)
					if err != nil {
						return nil, report, err
					}
					row[vars[i]] = t
				}
				result.Bindings = append(result.Bindings, row)
				if len(result.Bindings) > 100000 {
					return nil, report, fmt.Errorf("más de 100.000 resultados; agregá LIMIT")
				}
			}
			if err = rows.Err(); err != nil {
				return nil, report, err
			}
			report.DurationMS = float64(time.Since(started).Microseconds()) / 1000
			return result, report, nil
		}
	}
	vs := &virtualStore{MemoryStore: store.NewMemoryStore(), db: tx, relations: rels, base: s.Base}
	def := graph.NewGraph(graph.WithStore(vs))
	q.NamedGraphs = map[string]*graph.Graph{}
	for _, t := range schema.Tables {
		iri := s.Base + "graph/" + urlPart(t.Name)
		q.NamedGraphs[iri] = graph.NewGraph(graph.WithStore(vs), graph.WithIdentifier(term.NewURIRefUnsafe(iri)))
	}
	if len(q.DatasetClause) > 0 {
		selected := []relation{}
		named := map[string]*graph.Graph{}
		for _, c := range q.DatasetClause {
			g, ok := q.NamedGraphs[c.IRI]
			if !ok {
				return nil, report, fmt.Errorf("FROM referencia un grafo que no está mapeado: %s", c.IRI)
			}
			if c.Named {
				named[c.IRI] = g
			} else {
				for _, r := range rels {
					if s.Base+"graph/"+urlPart(r.table) == c.IRI {
						selected = append(selected, r)
					}
				}
			}
		}
		def = graph.NewGraph(graph.WithStore(&virtualStore{MemoryStore: store.NewMemoryStore(), db: tx, relations: selected, base: s.Base}))
		q.NamedGraphs = named
	}
	ctx = sparql.WithMaxSolutions(ctx, 200000)
	var result *sparql.Result
	if q.Limit == 0 {
		result = emptyResult(q)
	} else {
		result, e = sparql.EvalQueryContext(ctx, def, q, nil)
	}
	report.DurationMS = float64(time.Since(started).Microseconds()) / 1000
	return result, report, e
}

func flatten(p sparql.Pattern) ([]sparql.Triple, bool) {
	switch p := p.(type) {
	case *sparql.BGP:
		return p.Triples, true
	case *sparql.JoinPattern:
		l, ok := flatten(p.Left)
		if !ok {
			return nil, false
		}
		r, ok := flatten(p.Right)
		return append(l, r...), ok
	default:
		return nil, false
	}
}
func compile(q *sparql.ParsedQuery, rels []relation) (string, []any, []string, bool) {
	if sql, args, vars, ok := compileDirect(q, rels); ok {
		return sql, args, vars, true
	}
	if q.Type != "SELECT" && q.Type != "ASK" || len(q.OrderBy) > 0 || len(q.ProjectExprs) > 0 || len(q.GroupBy) > 0 || q.Having != nil || len(q.DatasetClause) > 0 {
		return "", nil, nil, false
	}
	triples, ok := flatten(q.Where)
	if !ok || len(triples) == 0 || len(triples) > 12 {
		return "", nil, nil, false
	}
	variables := map[string]string{}
	tables := []string{}
	conditions := []string{}
	args := []any{}
	for i, t := range triples {
		if t.PredicatePath != nil {
			return "", nil, nil, false
		}
		alias := "q" + strconv.Itoa(i)
		ps := []string{}
		pred := ""
		if !strings.HasPrefix(t.Predicate, "?") {
			pt, ok := resolve(t.Predicate, q.Prefixes)
			if !ok {
				return "", nil, nil, false
			}
			u, ok := pt.(term.URIRef)
			if !ok {
				return "", nil, nil, false
			}
			pred = u.Value()
		}
		for _, r := range rels {
			if pred == "" || pred == r.predicate {
				ps = append(ps, r.sql)
			}
		}
		if len(ps) == 0 {
			ps = []string{"SELECT '' s,'' p,'' o WHERE 0"}
		}
		tables = append(tables, "(SELECT DISTINCT s,p,o FROM ("+strings.Join(ps, " UNION ALL ")+")) "+alias)
		for k, raw := range []string{t.Subject, t.Predicate, t.Object} {
			col := alias + "." + []string{"s", "p", "o"}[k]
			if strings.HasPrefix(raw, "?") {
				v := strings.TrimPrefix(raw, "?")
				if old, found := variables[v]; found {
					conditions = append(conditions, col+"="+old)
				} else {
					variables[v] = col
				}
			} else {
				if strings.HasPrefix(raw, "_:") {
					return "", nil, nil, false
				}
				value, ok := resolve(raw, q.Prefixes)
				if !ok {
					return "", nil, nil, false
				}
				conditions = append(conditions, col+"=?")
				args = append(args, term.TermKey(value))
			}
		}
	}
	vars := append([]string{}, q.Variables...)
	if len(vars) == 0 {
		for v := range variables {
			vars = append(vars, v)
		}
		sort.Strings(vars)
	}
	selection := []string{}
	for _, v := range vars {
		col, ok := variables[v]
		if !ok {
			return "", nil, nil, false
		}
		selection = append(selection, col)
	}
	modifier := ""
	if q.Distinct {
		modifier = "DISTINCT "
	}
	if q.Type == "ASK" {
		selection = []string{"1"}
		vars = []string{}
	}
	query := "SELECT " + modifier + strings.Join(selection, ",") + " FROM " + strings.Join(tables, " JOIN ")
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	if q.Type == "ASK" {
		if q.Limit == 0 {
			query += " LIMIT 0"
		} else {
			query += " LIMIT 1"
		}
	} else if q.Limit >= 0 {
		query += " LIMIT " + strconv.Itoa(q.Limit)
	} else {
		query += " LIMIT -1"
	}
	if q.Offset > 0 {
		query += " OFFSET " + strconv.Itoa(q.Offset)
	}
	return query, args, vars, true
}
func resolve(raw string, prefixes map[string]string) (term.Term, bool) {
	if strings.Contains(raw, "\\") && !strings.HasPrefix(raw, "\"") {
		return nil, false
	}
	if raw == "a" {
		return term.NewURIRefUnsafe(term.RDFNamespace + "type"), true
	}
	if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
		iri := raw[1 : len(raw)-1]
		if !term.ValidIRI(iri) {
			return nil, false
		}
		return term.NewURIRefUnsafe(iri), true
	}
	if !strings.HasPrefix(raw, "\"") {
		if p, local, ok := strings.Cut(raw, ":"); ok {
			if ns, found := prefixes[p]; found {
				return term.NewURIRefUnsafe(ns + local), true
			}
		}
	}
	// N-Triples preserves literal lexical forms, languages and datatypes.
	if strings.HasPrefix(raw, "\"") {
		var value term.Term
		err := nt.ParseStream(strings.NewReader("<urn:s> <urn:p> "+raw+" ."), func(_ term.Subject, _ term.URIRef, o term.Term) error { value = o; return nil })
		return value, err == nil && value != nil
	}
	if raw == "true" || raw == "false" {
		return term.NewLiteral(raw == "true"), true
	}
	if n, e := strconv.ParseInt(raw, 10, 64); e == nil {
		return term.NewLiteral(n), true
	}
	return nil, false // Defer decimals, expressions and escaped prefixed names.
}

// EvaluateGraph is shared by the educational app; no SQL source is required.
func EvaluateGraph(ctx context.Context, g *graph.Graph, text string) (*sparql.Result, error) {
	q, e := sparql.Parse(text)
	if e != nil {
		return nil, e
	}
	if q.Limit == 0 {
		return emptyResult(q), nil
	}
	return sparql.EvalQueryContext(sparql.WithMaxSolutions(ctx, 200000), g, q, nil)
}
func emptyResult(q *sparql.ParsedQuery) *sparql.Result {
	r := &sparql.Result{Type: q.Type, Vars: q.Variables, Bindings: []map[string]term.Term{}}
	if q.Type == "CONSTRUCT" || q.Type == "DESCRIBE" {
		r.Graph = graph.NewGraph()
	}
	return r
}
