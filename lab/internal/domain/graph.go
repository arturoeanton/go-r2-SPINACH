package domain

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
)

const Namespace = "https://spinach.local/"
const RDFSLabel = "http://www.w3.org/2000/01/rdf-schema#label"

var ErrNotFound = errors.New("grafo no encontrado")
var ErrConflict = errors.New("el grafo cambió en otra ventana; recargá antes de guardar")

type Node struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Kind     string  `json:"kind"`
	Datatype string  `json:"datatype,omitempty"`
	Language string  `json:"language,omitempty"`
	Group    string  `json:"group"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
}
type Edge struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Predicate string `json:"predicate"`
	Label     string `json:"label"`
}
type Subgraph struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Nodes       []string `json:"nodes"`
}
type Example struct {
	Question string `json:"question"`
	Query    string `json:"query"`
}
type Graph struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Nodes        []Node     `json:"nodes"`
	Edges        []Edge     `json:"edges"`
	Subgraphs    []Subgraph `json:"subgraphs"`
	Examples     []Example  `json:"examples"`
	Revision     int64      `json:"revision"`
	SQLiteSource string     `json:"sqliteSource,omitempty"`
}

// Repository is the only persistence contract consumed by the application.
type Repository interface {
	List(context.Context) ([]Graph, error)
	Get(context.Context, string) (Graph, error)
	Save(context.Context, *Graph) error
	Delete(context.Context, string) error
}

func IRI(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.IsAbs() && !strings.ContainsAny(s, " <>\"{}|^`\\\n\r\t")
}
func (g Graph) Validate() error {
	if len(g.ID) == 0 || len(g.ID) > 100 || strings.ContainsAny(g.ID, "/\\") {
		return errors.New("ID de grafo inválido")
	}
	if strings.TrimSpace(g.Title) == "" || len(g.Title) > 160 {
		return errors.New("el título debe tener entre 1 y 160 caracteres")
	}
	if len(g.Description) > 4000 || len(g.Nodes) > 400 || len(g.Edges) > 2000 || len(g.Subgraphs) > 24 || len(g.Examples) > 30 {
		return errors.New("límite educativo: 400 nodos, 2000 relaciones, 24 subgrafos y 30 ejemplos")
	}
	nodes := map[string]Node{}
	for _, n := range g.Nodes {
		if n.Kind != "iri" && n.Kind != "literal" && n.Kind != "bnode" {
			return fmt.Errorf("tipo inválido en %s: usar iri, bnode o literal", n.ID)
		}
		if n.ID == "" || len(n.ID) > 1000 || len(n.Label) > 2000 || len(n.Group) > 100 || math.IsNaN(n.X) || math.IsNaN(n.Y) || math.IsInf(n.X, 0) || math.IsInf(n.Y, 0) || math.Abs(n.X) > 10000 || math.Abs(n.Y) > 10000 {
			return errors.New("nodo inválido o coordenadas fuera de rango")
		}
		if n.Datatype != "" && !IRI(n.Datatype) {
			return fmt.Errorf("datatype inválido: %s", n.Datatype)
		}
		if n.Kind == "iri" && !IRI(n.ID) {
			return fmt.Errorf("IRI inválido: %s", n.ID)
		}
		if _, ok := nodes[n.ID]; ok {
			return fmt.Errorf("nodo duplicado: %s", n.ID)
		}
		nodes[n.ID] = n
	}
	edges := map[string]bool{}
	for _, e := range g.Edges {
		s, ok := nodes[e.Source]
		if !ok || s.Kind == "literal" {
			return fmt.Errorf("sujeto inexistente o literal: %s", e.Source)
		}
		if _, ok := nodes[e.Target]; !ok {
			return fmt.Errorf("destino inexistente: %s", e.Target)
		}
		if !IRI(e.Predicate) || e.ID == "" || len(e.ID) > 100 || len(e.Label) > 200 || edges[e.ID] {
			return errors.New("relación inválida o ID repetido")
		}
		edges[e.ID] = true
	}
	subs := map[string]bool{}
	for _, s := range g.Subgraphs {
		if s.ID == "" || subs[s.ID] || strings.TrimSpace(s.Name) == "" || len(s.Name) > 120 || len(s.Description) > 1000 || len(s.Nodes) > 400 {
			return errors.New("subgrafo inválido o repetido")
		}
		subs[s.ID] = true
		for _, id := range s.Nodes {
			if _, ok := nodes[id]; !ok {
				return fmt.Errorf("subgrafo contiene nodo inexistente: %s", id)
			}
		}
	}
	for _, e := range g.Examples {
		if len(e.Question) > 2000 || len(e.Query) > 16000 {
			return errors.New("ejemplo demasiado extenso")
		}
	}
	return nil
}
