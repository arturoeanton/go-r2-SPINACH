package sparql

import (
	"context"
	"sort"
	"time"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
	"github.com/arturoeanton/go-r2-SPINACH/sparqlite"
	"github.com/tggo/goRDFlib/graph"
	engine "github.com/tggo/goRDFlib/sparql"
	"github.com/tggo/goRDFlib/term"
)

type Cell struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Datatype string `json:"datatype,omitempty"`
	Language string `json:"language,omitempty"`
	NodeID   string `json:"nodeId,omitempty"`
}
type Result struct {
	Variables  []string          `json:"variables"`
	Rows       []map[string]Cell `json:"rows"`
	Boolean    *bool             `json:"boolean,omitempty"`
	EdgeIDs    []string          `json:"edgeIds"`
	NodeIDs    []string          `json:"nodeIds"`
	Truncated  bool              `json:"truncated"`
	Engine     string            `json:"engine"`
	DurationMS float64           `json:"durationMs"`
	SQL        string            `json:"sql,omitempty"`
	Plan       []string          `json:"plan,omitempty"`
}

func NodeTerm(n domain.Node) term.Term {
	switch n.Kind {
	case "literal":
		opts := []term.LiteralOption{}
		if n.Datatype != "" {
			opts = append(opts, term.WithDatatype(term.NewURIRefUnsafe(n.Datatype)))
		}
		if n.Language != "" {
			opts = append(opts, term.WithLang(n.Language))
		}
		return term.NewLiteral(n.Label, opts...)
	case "bnode":
		return term.NewBNode(n.ID)
	default:
		return term.NewURIRefUnsafe(n.ID)
	}
}
func RDFGraph(ctx context.Context, g domain.Graph) (*graph.Graph, error) {
	rdf := graph.NewGraph()
	nodes := map[string]term.Term{}
	for _, n := range g.Nodes {
		nodes[n.ID] = NodeTerm(n)
		if s, ok := nodes[n.ID].(term.Subject); ok {
			if err := rdf.Add(ctx, s, term.NewURIRefUnsafe(domain.RDFSLabel), term.NewLiteral(n.Label)); err != nil {
				return nil, err
			}
		}
	}
	for _, e := range g.Edges {
		if err := rdf.Add(ctx, nodes[e.Source].(term.Subject), term.NewURIRefUnsafe(e.Predicate), nodes[e.Target]); err != nil {
			return nil, err
		}
	}
	if err := rdf.Bind(ctx, "ex", term.NewURIRefUnsafe(domain.Namespace)); err != nil {
		return nil, err
	}
	return rdf, nil
}
func Evaluate(ctx context.Context, g domain.Graph, text string) (Result, error) {
	start := time.Now()
	rdf, err := RDFGraph(ctx, g)
	if err != nil {
		return Result{}, err
	}
	text = "PREFIX ex: <https://spinach.local/>\nPREFIX rdfs: <http://www.w3.org/2000/01/rdf-schema#>\nPREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>\nPREFIX xsd: <http://www.w3.org/2001/XMLSchema#>\n" + text
	res, err := sparqlite.EvaluateGraph(ctx, rdf, text)
	if err != nil {
		return Result{}, err
	}
	r := Convert(ctx, res, g)
	r.Engine = "goRDFlib 0.9.3"
	r.DurationMS = float64(time.Since(start).Microseconds()) / 1000
	return r, nil
}
func Convert(ctx context.Context, res *engine.Result, g domain.Graph) Result {
	r := Result{Variables: append([]string{}, res.Vars...), Rows: []map[string]Cell{}, EdgeIDs: []string{}, NodeIDs: []string{}}
	byKey := map[string]domain.Node{}
	for _, n := range g.Nodes {
		byKey[term.TermKey(NodeTerm(n))] = n
	}
	seen := map[string]bool{}
	cell := func(t term.Term) Cell {
		c := Cell{Value: t.String(), Label: t.String(), Kind: "iri"}
		switch v := t.(type) {
		case term.Literal:
			c.Value = v.Lexical()
			c.Label = v.Lexical()
			c.Kind = "literal"
			c.Datatype = v.Datatype().Value()
			c.Language = v.Language()
		case term.BNode:
			c.Kind = "bnode"
		}
		if n, ok := byKey[term.TermKey(t)]; ok {
			c.NodeID = n.ID
			c.Label = n.Label
			seen[n.ID] = true
		}
		return c
	}
	if res.Type == "ASK" {
		v := res.AskResult
		r.Boolean = &v
	}
	for i, row := range res.Bindings {
		if i >= 500 {
			r.Truncated = true
			break
		}
		out := map[string]Cell{}
		for k, t := range row {
			if t != nil {
				out[k] = cell(t)
			}
		}
		r.Rows = append(r.Rows, out)
	}
	if res.Graph != nil {
		r.Variables = []string{"s", "p", "o"}
		count := 0
		for tr, err := range res.Graph.Triples(ctx, nil, nil, nil) {
			if err != nil {
				break
			}
			if count >= 500 {
				r.Truncated = true
				break
			}
			r.Rows = append(r.Rows, map[string]Cell{"s": cell(tr.Subject), "p": cell(tr.Predicate), "o": cell(tr.Object)})
			count++
		}
	}
	for id := range seen {
		r.NodeIDs = append(r.NodeIDs, id)
	}
	sort.Strings(r.NodeIDs)
	return r
}
