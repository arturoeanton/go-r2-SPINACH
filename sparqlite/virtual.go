package sparqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/tggo/goRDFlib/store"
	"github.com/tggo/goRDFlib/term"
)

type virtualStore struct {
	*store.MemoryStore
	db        querier
	relations []relation
	base      string
}

func (v *virtualStore) ContextAware() bool     { return true }
func (v *virtualStore) TransactionAware() bool { return false }
func (v *virtualStore) Add(context.Context, term.Triple, term.Term) error {
	return fmt.Errorf("mapeo relacional de solo lectura: SPARQL Update no tiene una traducción relacional inequívoca")
}
func (v *virtualStore) AddN(context.Context, []term.Quad) error {
	return fmt.Errorf("mapeo de solo lectura")
}
func (v *virtualStore) Remove(context.Context, term.TriplePattern, term.Term) error {
	return fmt.Errorf("mapeo de solo lectura")
}
func (v *virtualStore) Set(context.Context, term.Triple, term.Term) error {
	return fmt.Errorf("mapeo de solo lectura")
}
func (v *virtualStore) source(p term.TriplePattern, g term.Term) (string, []any) {
	selections := []string{}
	for _, r := range v.relations {
		if p.Predicate != nil && p.Predicate.Value() != r.predicate {
			continue
		}
		if !store.IsDefaultGraph(g) {
			if u, ok := g.(term.URIRef); !ok || u.Value() != v.base+"graph/"+urlPart(r.table) {
				continue
			}
		}
		selections = append(selections, r.sql)
	}
	if len(selections) == 0 {
		return "SELECT '' s,'' p,'' o WHERE 0", nil
	}
	conditions := []string{}
	args := []any{}
	for _, c := range []struct {
		name string
		t    term.Term
	}{{"s", p.Subject}, {"p", predTerm(p.Predicate)}, {"o", p.Object}} {
		if c.t != nil {
			conditions = append(conditions, c.name+"=?")
			args = append(args, term.TermKey(c.t))
		}
	}
	query := "SELECT DISTINCT s,p,o FROM (" + strings.Join(selections, " UNION ALL ") + ")"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	return query, args
}
func predTerm(p *term.URIRef) term.Term {
	if p == nil {
		return nil
	}
	return *p
}
func (v *virtualStore) Triples(ctx context.Context, p term.TriplePattern, g term.Term) store.TripleIterator {
	return func(yield func(term.Triple, error) bool) {
		query, args := v.source(p, g)
		rows, err := v.db.QueryContext(ctx, query, args...)
		if err != nil {
			yield(term.Triple{}, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var s, p, o string
			if err = rows.Scan(&s, &p, &o); err != nil {
				yield(term.Triple{}, err)
				return
			}
			tr, err := decodeTriple(s, p, o)
			if !yield(tr, err) || err != nil {
				return
			}
		}
		if err = rows.Err(); err != nil {
			yield(term.Triple{}, err)
		}
	}
}
func (v *virtualStore) Len(ctx context.Context, g term.Term) (int, error) {
	return v.Count(ctx, term.TriplePattern{}, g)
}
func (v *virtualStore) Count(ctx context.Context, p term.TriplePattern, g term.Term) (int, error) {
	q, a := v.source(p, g)
	var n int
	err := v.db.QueryRowContext(ctx, "SELECT count(*) FROM ("+q+")", a...).Scan(&n)
	return n, err
}
func (v *virtualStore) Exists(ctx context.Context, p term.TriplePattern, g term.Term) (bool, error) {
	q, a := v.source(p, g)
	var n int
	err := v.db.QueryRowContext(ctx, "SELECT 1 FROM ("+q+") LIMIT 1", a...).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return n == 1, err
}
func (v *virtualStore) TriplesWithLimit(ctx context.Context, p term.TriplePattern, g term.Term, limit, offset int) store.TripleIterator {
	return func(yield func(term.Triple, error) bool) {
		i, n := 0, 0
		for tr, err := range v.Triples(ctx, p, g) {
			if err != nil {
				yield(tr, err)
				return
			}
			if i < offset {
				i++
				continue
			}
			if !yield(tr, nil) {
				return
			}
			n++
			if limit > 0 && n >= limit {
				return
			}
		}
	}
}
func (v *virtualStore) Contexts(ctx context.Context, tr *term.Triple) store.TermIterator {
	return func(yield func(term.Term, error) bool) {
		seen := map[string]bool{}
		for _, r := range v.relations {
			if seen[r.table] {
				continue
			}
			seen[r.table] = true
			g := term.NewURIRefUnsafe(v.base + "graph/" + urlPart(r.table))
			if tr != nil {
				p := tr.Predicate
				ok, err := v.Exists(ctx, term.TriplePattern{Subject: tr.Subject, Predicate: &p, Object: tr.Object}, g)
				if err != nil {
					yield(nil, err)
					return
				}
				if !ok {
					continue
				}
			}
			if !yield(g, nil) {
				return
			}
		}
	}
}
func decodeTriple(s, p, o string) (term.Triple, error) {
	st, e := term.TermFromKey(s)
	if e != nil {
		return term.Triple{}, e
	}
	pt, e := term.TermFromKey(p)
	if e != nil {
		return term.Triple{}, e
	}
	ot, e := term.TermFromKey(o)
	if e != nil {
		return term.Triple{}, e
	}
	sub, ok := st.(term.Subject)
	if !ok {
		return term.Triple{}, fmt.Errorf("sujeto inválido")
	}
	pred, ok := pt.(term.URIRef)
	if !ok {
		return term.Triple{}, fmt.Errorf("predicado inválido")
	}
	return term.Triple{sub, pred, ot}, nil
}
