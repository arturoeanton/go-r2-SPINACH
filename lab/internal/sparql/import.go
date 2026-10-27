package sparql

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
	"github.com/tggo/goRDFlib/graph"
	"github.com/tggo/goRDFlib/nt"
	"github.com/tggo/goRDFlib/rdfxml"
	"github.com/tggo/goRDFlib/term"
	"github.com/tggo/goRDFlib/turtle"
)

func ImportRDF(ctx context.Context, id, title, text, format string) (domain.Graph, error) {
	g := graph.NewGraph()
	var err error
	switch format {
	case "nt":
		err = nt.Parse(ctx, g, strings.NewReader(text))
	case "ttl":
		err = turtle.Parse(ctx, g, strings.NewReader(text))
	case "rdfxml":
		err = rdfxml.Parse(ctx, g, strings.NewReader(text))
	default:
		err = fmt.Errorf("formato RDF no soportado")
	}
	if err != nil {
		return domain.Graph{}, err
	}
	return FromRDF(ctx, id, title, g)
}
func FromRDF(ctx context.Context, id, title string, rdf *graph.Graph) (domain.Graph, error) {
	g := domain.Graph{ID: id, Title: title, Description: "Grafo RDF importado. Podés editar las entidades y sus relaciones.", Nodes: []domain.Node{}, Edges: []domain.Edge{}, Subgraphs: []domain.Subgraph{}, Examples: []domain.Example{}}
	lookup := map[string]string{}
	labels := map[string]string{}
	labelPred := term.NewURIRefUnsafe(domain.RDFSLabel)
	for tr, err := range rdf.Triples(ctx, nil, &labelPred, nil) {
		if err != nil {
			return g, err
		}
		if l, ok := tr.Object.(term.Literal); ok {
			labels[term.TermKey(tr.Subject)] = l.Lexical()
		}
	}
	node := func(t term.Term) string {
		k := term.TermKey(t)
		if v, ok := lookup[k]; ok {
			return v
		}
		nid := t.String()
		n := domain.Node{ID: nid, Label: Local(nid), Kind: "iri", Group: "Entidades", X: 100 + float64(len(g.Nodes)%5)*170, Y: 100 + float64(len(g.Nodes)/5)*130}
		switch v := t.(type) {
		case term.Literal:
			h := sha256.Sum256([]byte(k))
			nid = fmt.Sprintf("literal:%x", h[:12])
			n.ID = nid
			n.Label = v.Lexical()
			n.Kind = "literal"
			n.Group = "Valores"
			n.Datatype = v.Datatype().Value()
			n.Language = v.Language()
		case term.BNode:
			n.Kind = "bnode"
			n.Group = "Nodos anónimos"
		}
		if l, ok := labels[k]; ok {
			n.Label = l
		}
		lookup[k] = nid
		g.Nodes = append(g.Nodes, n)
		return nid
	}
	for tr, err := range rdf.Triples(ctx, nil, nil, nil) {
		if err != nil {
			return g, err
		}
		s, o := node(tr.Subject), node(tr.Object)
		g.Edges = append(g.Edges, domain.Edge{ID: fmt.Sprintf("import-%d", len(g.Edges)+1), Source: s, Target: o, Predicate: tr.Predicate.Value(), Label: Local(tr.Predicate.Value())})
		if len(g.Nodes) > 400 || len(g.Edges) > 2000 {
			return g, fmt.Errorf("el editor admite 400 nodos y 2000 relaciones; usá una vista parcial para grafos mayores")
		}
	}
	return g, g.Validate()
}
func Local(iri string) string {
	if i := strings.LastIndexAny(iri, "/#:"); i >= 0 && i < len(iri)-1 {
		return iri[i+1:]
	}
	return iri
}
