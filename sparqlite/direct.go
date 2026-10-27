package sparqlite

import (
	"sort"
	"strconv"
	"strings"

	"github.com/tggo/goRDFlib/sparql"
	"github.com/tggo/goRDFlib/term"
)

// directTerm retains the physical identity of an RDF term. This lets an RDF
// subject join become an indexed primary-key SQL join, without constructing
// and comparing URI strings for every candidate row.
type directTerm struct {
	expr, raw string
	identity  *Table
	keys      []string
}

func compileDirect(q *sparql.ParsedQuery, rels []relation) (string, []any, []string, bool) {
	if q.Type != "SELECT" && q.Type != "ASK" || len(q.OrderBy) > 0 || len(q.ProjectExprs) > 0 || len(q.GroupBy) > 0 || q.Having != nil || len(q.DatasetClause) > 0 {
		return "", nil, nil, false
	}
	triples, ok := flatten(q.Where)
	if !ok || len(triples) == 0 || len(triples) > 12 {
		return "", nil, nil, false
	}
	vars := map[string]directTerm{}
	from := []string{}
	where := []string{}
	args := []any{}
	for i, tp := range triples {
		if tp.PredicatePath != nil || strings.HasPrefix(tp.Predicate, "?") {
			return "", nil, nil, false
		}
		pred, ok := resolve(tp.Predicate, q.Prefixes)
		if !ok {
			return "", nil, nil, false
		}
		pu, ok := pred.(term.URIRef)
		if !ok {
			return "", nil, nil, false
		}
		candidates := []relation{}
		for _, r := range rels {
			if r.predicate == pu.Value() {
				candidates = append(candidates, r)
			}
		}
		if len(candidates) != 1 {
			return "", nil, nil, false
		}
		r := candidates[0]
		rn, tn := "r"+strconv.Itoa(i), "t"+strconv.Itoa(i)
		replace := func(v string) string { return strings.ReplaceAll(strings.ReplaceAll(v, "r.", rn+"."), "t.", tn+".") }
		fr := replace(r.from)
		fr = strings.ReplaceAll(fr, " r JOIN ", " "+rn+" JOIN ")
		if strings.HasSuffix(fr, " r") {
			fr = strings.TrimSuffix(fr, " r") + " " + rn
		}
		fr = strings.ReplaceAll(fr, " t ON ", " "+tn+" ON ")
		from = append(from, fr)
		where = append(where, replace(r.where))
		subject := directTerm{expr: replace(r.subject), identity: r.identity}
		if len(r.identity.pk) == 0 {
			return "", nil, nil, false
		}
		for _, key := range r.identity.pk {
			subject.keys = append(subject.keys, rn+"."+qi(key))
		}
		object := directTerm{expr: replace(r.object)}
		if r.column != "" {
			object.raw = rn + "." + qi(r.column)
		}
		if r.objectIdentity != nil {
			object.identity = r.objectIdentity
			if len(object.identity.pk) == 0 {
				return "", nil, nil, false
			}
			for _, key := range object.identity.pk {
				object.keys = append(object.keys, tn+"."+qi(key))
			}
		}
		for k, raw := range []string{tp.Subject, tp.Predicate, tp.Object} {
			dt := []directTerm{subject, {expr: lit(iriKey(pu.Value()))}, object}[k]
			if strings.HasPrefix(raw, "?") {
				name := strings.TrimPrefix(raw, "?")
				if old, exists := vars[name]; exists {
					if old.identity != nil && dt.identity != nil && old.identity.Name == dt.identity.Name && len(old.keys) == len(dt.keys) {
						for i := range old.keys {
							where = append(where, old.keys[i]+" IS "+dt.keys[i])
						}
						where = append(where, old.expr+"="+dt.expr)
					} else {
						if old.raw != "" && dt.raw != "" {
							where = append(where, old.raw+"="+dt.raw)
						}
						where = append(where, old.expr+"="+dt.expr)
					}
				} else {
					vars[name] = dt
				}
			} else {
				if strings.HasPrefix(raw, "_:") {
					return "", nil, nil, false
				}
				value, ok := resolve(raw, q.Prefixes)
				if !ok {
					return "", nil, nil, false
				}
				if literal, ok := value.(term.Literal); ok && dt.raw != "" {
					switch v := literal.Value().(type) {
					case string, int64, float64, bool, int:
						where = append(where, dt.raw+"=?")
						args = append(args, v)
					}
				}
				where = append(where, dt.expr+"=?")
				args = append(args, term.TermKey(value))
			}
		}
	}
	projection := append([]string{}, q.Variables...)
	if len(projection) == 0 {
		for v := range vars {
			projection = append(projection, v)
		}
		sort.Strings(projection)
	}
	cols := []string{}
	for _, v := range projection {
		dt, ok := vars[v]
		if !ok {
			return "", nil, nil, false
		}
		cols = append(cols, dt.expr)
	}
	if len(cols) == 0 {
		return "", nil, nil, false
	}
	distinct := ""
	if q.Distinct {
		distinct = "DISTINCT "
	}
	if q.Type == "ASK" {
		cols = []string{"1"}
		projection = []string{}
	}
	query := "SELECT " + distinct + strings.Join(cols, ",") + " FROM " + strings.Join(from, " JOIN ") + " WHERE " + strings.Join(where, " AND ")
	limit := q.Limit
	if q.Type == "ASK" && limit != 0 {
		limit = 1
	}
	query += " LIMIT " + strconv.Itoa(limit)
	if q.Offset > 0 {
		query += " OFFSET " + strconv.Itoa(q.Offset)
	}
	return query, args, projection, true
}
