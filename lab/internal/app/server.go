package app

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tggo/goRDFlib/graph"
	"github.com/tggo/goRDFlib/term"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/models"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/sparql"
	"github.com/arturoeanton/go-r2-SPINACH/sparqlite"
)

//go:embed web/*
var web embed.FS

type SourceRegistry interface {
	Open(string) (*sparqlite.Source, error)
	Register(context.Context, string, []byte) (sparqlite.Schema, error)
}
type Server struct {
	Repo      domain.Repository
	Sources   SourceRegistry
	Decisions models.DecisionModel
	Language  models.LanguageModel
	Local     *models.Local
	Mux       *http.ServeMux
}

func New(repo domain.Repository, local *models.Local, sources SourceRegistry) *Server {
	s := &Server{Repo: repo, Sources: sources, Decisions: local, Language: local, Local: local, Mux: http.NewServeMux()}
	s.Mux.HandleFunc("GET /api/graphs", s.list)
	s.Mux.HandleFunc("POST /api/graphs", s.create)
	s.Mux.HandleFunc("GET /api/graphs/{id}", s.get)
	s.Mux.HandleFunc("PUT /api/graphs/{id}", s.save)
	s.Mux.HandleFunc("DELETE /api/graphs/{id}", s.remove)
	s.Mux.HandleFunc("POST /api/graphs/{id}/query", s.query)
	s.Mux.HandleFunc("POST /api/graphs/{id}/route", s.route)
	s.Mux.HandleFunc("POST /api/graphs/{id}/step", s.step)
	s.Mux.HandleFunc("POST /api/import", s.importGraph)
	s.Mux.HandleFunc("POST /api/databases", s.importDatabase)
	s.Mux.HandleFunc("GET /api/databases/{id}/schema", s.databaseSchema)
	s.Mux.HandleFunc("GET /api/status", s.status)
	s.Mux.HandleFunc("GET /api/examples", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, domain.Seeds()) })
	sub, _ := fs.Sub(web, "web")
	s.Mux.Handle("GET /", http.FileServer(http.FS(sub)))
	return s
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if recover() != nil {
			fail(w, 500, errors.New("falló la operación; no se obtuvo un resultado válido"))
		}
	}()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					fail(w, 403, errors.New("origen no permitido"))
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				fail(w, 403, errors.New("origen no permitido"))
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				fail(w, 415, errors.New("se requiere application/json"))
				return
			}
		}
	}
	s.Mux.ServeHTTP(w, r)
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, err error) {
	reply(w, status, map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 24<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, fmt.Errorf("JSON inválido: %w", err))
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, errors.New("se esperaba un único objeto JSON"))
		return false
	}
	return true
}
func repoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		fail(w, 404, err)
	case errors.Is(err, domain.ErrConflict):
		fail(w, 409, err)
	default:
		fail(w, 500, errors.New("no se pudo acceder al almacenamiento"))
	}
}
func (s *Server) graph(w http.ResponseWriter, r *http.Request) (domain.Graph, bool) {
	g, err := s.Repo.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		repoError(w, err)
		return g, false
	}
	return g, true
}
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	gs, err := s.Repo.List(r.Context())
	if err != nil {
		repoError(w, err)
		return
	}
	reply(w, 200, gs)
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	g, ok := s.graph(w, r)
	if ok {
		reply(w, 200, g)
	}
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var g domain.Graph
	if !decode(w, r, &g) {
		return
	}
	g.ID = ID()
	g.Revision = 0
	s.store(w, r, &g, 201)
}
func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	var g domain.Graph
	if !decode(w, r, &g) {
		return
	}
	g.ID = r.PathValue("id")
	if g.Revision < 1 {
		fail(w, 400, errors.New("falta revisión para actualizar"))
		return
	}
	s.store(w, r, &g, 200)
}
func (s *Server) store(w http.ResponseWriter, r *http.Request, g *domain.Graph, status int) {
	if err := g.Validate(); err != nil {
		fail(w, 422, err)
		return
	}
	if err := s.Repo.Save(r.Context(), g); err != nil {
		repoError(w, err)
		return
	}
	reply(w, status, g)
}
func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	if err := s.Repo.Delete(r.Context(), r.PathValue("id")); err != nil {
		repoError(w, err)
		return
	}
	reply(w, 200, map[string]bool{"deleted": true})
}
func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Query string `json:"query"`
	}
	if !decode(w, r, &b) {
		return
	}
	g, ok := s.graph(w, r)
	if !ok {
		return
	}
	res, err := s.evaluate(r.Context(), g, b.Query)
	if err != nil {
		fail(w, 422, err)
		return
	}
	reply(w, 200, res)
}
func (s *Server) evaluate(ctx context.Context, g domain.Graph, text string) (sparql.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if g.SQLiteSource == "" {
		return sparql.Evaluate(ctx, g, text)
	}
	src, err := s.Sources.Open(g.SQLiteSource)
	if err != nil {
		return sparql.Result{}, err
	}
	res, report, err := src.Query(ctx, text)
	if err != nil {
		return sparql.Result{}, err
	}
	r := sparql.Convert(ctx, res, g)
	r.Engine = report.Engine
	r.SQL = report.SQL
	r.Plan = report.Plan
	r.DurationMS = report.DurationMS
	return r, nil
}

type request struct {
	Question        string         `json:"question"`
	Model           string         `json:"model"`
	Focus           string         `json:"focus"`
	Language        string         `json:"language,omitempty"`
	NavigationModel string         `json:"navigationModel,omitempty"`
	Events          []models.Event `json:"events"`
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	var b request
	if !decode(w, r, &b) {
		return
	}
	if !validRequest(w, b) {
		return
	}
	g, ok := s.graph(w, r)
	if !ok {
		return
	}
	d, err := s.Decisions.Rank(r.Context(), g, b.Question, b.Model)
	if err != nil {
		fail(w, 502, err)
		return
	}
	reply(w, 200, d)
}
func validRequest(w http.ResponseWriter, b request) bool {
	if strings.TrimSpace(b.Question) == "" || len(b.Question) > 2000 || strings.TrimSpace(b.Model) == "" || len(b.Model) > 160 || len(b.Events) > 8 {
		fail(w, 422, errors.New("se requieren pregunta y modelo válidos; máximo 8 pasos"))
		return false
	}
	return true
}
func (s *Server) step(w http.ResponseWriter, r *http.Request) {
	var b request
	if !decode(w, r, &b) {
		return
	}
	if !validRequest(w, b) {
		return
	}
	if len(b.Events) >= 8 {
		fail(w, 422, errors.New("se alcanzó el máximo de 8 pasos; revisá la evidencia o iniciá otro recorrido"))
		return
	}
	g, ok := s.graph(w, r)
	if !ok {
		return
	}
	// Rebuild observations from graph data. Client-supplied observations are never
	// treated as query evidence or instructions for the language model.
	events := []models.Event{}
	hasQuery := false
	for _, old := range b.Events {
		ev := s.execute(r.Context(), g, old.Action)
		if old.Action.Type == "answer" && !hasQuery {
			ev.Observation = nil
			ev.Error = "A successful query must run before an answer is allowed. Run a query now."
		}
		if ev.Action.Type == "query" && ev.Error == "" {
			hasQuery = true
		}
		events = append(events, ev)
	}
	navigationModel := b.NavigationModel
	if navigationModel == "" {
		navigationModel = "laya:multilingual"
	}
	navigationState := b.Question
	if len(events) > 0 {
		last := events[len(events)-1]
		navigationState += "\nPrevious exploration: " + last.Action.Type + ". " + last.Action.Objective
		if last.Error != "" {
			navigationState += "\nTool error: " + last.Error
		}
		if last.Action.Type == "query" {
			if result, ok := last.Observation.(sparql.Result); ok {
				navigationState += fmt.Sprintf("\nThe last query returned %d rows. Choose the next region to inspect, retaining useful regions when results are sufficient.", len(result.Rows))
			}
		}
	}
	navigation, err := s.Decisions.Rank(r.Context(), g, navigationState, navigationModel)
	if err != nil {
		fail(w, 502, err)
		return
	}
	a, err := s.Language.Next(r.Context(), g, b.Question, b.Model, navigation.Choice, b.Language, events)
	if err != nil {
		fail(w, 502, err)
		return
	}
	if a.Type == "answer" && !hasQuery {
		reply(w, 200, models.Event{Action: a, Error: "Antes de responder, ejecutá una consulta exitosa para obtener evidencia."})
		return
	}
	event := s.execute(r.Context(), g, a)
	event.Navigation = &navigation
	reply(w, 200, event)
}
func (s *Server) execute(ctx context.Context, g domain.Graph, a models.Action) models.Event {
	ev := models.Event{Action: a}
	switch a.Type {
	case "query":
		res, err := s.evaluate(ctx, g, a.Query)
		if err != nil {
			ev.Error = err.Error()
		} else {
			ev.Observation = res
		}
	case "search":
		matches := []domain.Node{}
		for _, n := range g.Nodes {
			if strings.Contains(strings.ToLower(n.Label+" "+n.ID), strings.ToLower(a.Text)) && len(matches) < 30 {
				matches = append(matches, n)
			}
		}
		ev.Observation = matches
	case "explore":
		found := false
		for _, n := range g.Nodes {
			if n.ID == a.NodeID {
				found = true
				break
			}
		}
		if !found {
			ev.Error = "nodo inexistente"
			break
		}
		edges := []domain.Edge{}
		for _, e := range g.Edges {
			if e.Source == a.NodeID || e.Target == a.NodeID {
				edges = append(edges, e)
			}
		}
		ev.Observation = edges
	case "answer":
		ev.Observation = map[string]string{"answer": a.Answer, "notice": "Síntesis generada por Gemma; verificá su correspondencia con la tabla de evidencia."}
	default:
		ev.Error = "acción no admitida"
	}
	return ev
}
func (s *Server) importGraph(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Format  string `json:"format"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if !decode(w, r, &b) {
		return
	}
	var g domain.Graph
	var err error
	switch b.Format {
	case "json":
		err = json.Unmarshal([]byte(b.Content), &g)
		g.ID = ID()
		g.Revision = 0
		if b.Title != "" {
			g.Title = b.Title
		}
	case "nt", "ttl", "rdfxml":
		g, err = sparql.ImportRDF(r.Context(), ID(), b.Title, b.Content, b.Format)
	default:
		err = errors.New("formatos admitidos: json, nt, ttl o rdfxml")
	}
	if err != nil {
		fail(w, 422, err)
		return
	}
	s.store(w, r, &g, 201)
}
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	type result struct {
		name   string
		status models.ServiceStatus
	}
	ch := make(chan result, 2)
	go func() { ch <- result{"ollama", s.Local.LanguageStatus(r.Context())} }()
	go func() { ch <- result{"ollaya", s.Local.Status(r.Context(), s.Local.OllayaURL)} }()
	model := "google/gemma-4-12b"
	if s.Local.Provider == "ollama" {
		model = "gemma4:12b"
	}
	out := map[string]any{"provider": s.Local.Provider, "defaults": map[string]string{"ollama": model, "ollaya": "laya:multilingual"}}
	for range 2 {
		v := <-ch
		out[v.name] = v.status
	}
	reply(w, 200, out)
}
func ID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (s *Server) databaseSchema(w http.ResponseWriter, r *http.Request) {
	src, e := s.Sources.Open(r.PathValue("id"))
	if e != nil {
		fail(w, 404, e)
		return
	}
	schema, e := src.Inspect(r.Context())
	if e != nil {
		fail(w, 422, e)
		return
	}
	reply(w, 200, schema)
}
func (s *Server) importDatabase(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Title   string `json:"title"`
		Content []byte `json:"content"`
	}
	if !decode(w, r, &b) {
		return
	}
	id := ID()
	schema, e := s.Sources.Register(r.Context(), id, b.Content)
	if e != nil {
		fail(w, 422, e)
		return
	}
	src, e := s.Sources.Open(id)
	if e != nil {
		fail(w, 422, e)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	res, _, e := src.Query(ctx, "SELECT ?s ?p ?o WHERE { ?s ?p ?o } LIMIT 80")
	if e != nil {
		fail(w, 422, e)
		return
	}
	rdf := graph.NewGraph()
	for _, row := range res.Bindings {
		sub, ok := row["s"].(term.Subject)
		if !ok {
			continue
		}
		pred, ok := row["p"].(term.URIRef)
		if !ok {
			continue
		}
		if e = rdf.Add(ctx, sub, pred, row["o"]); e != nil {
			fail(w, 422, e)
			return
		}
	}
	title := strings.TrimSpace(b.Title)
	if title == "" {
		title = "Mi base SQLite"
	}
	g, e := sparql.FromRDF(ctx, ID(), title, rdf)
	if e != nil {
		fail(w, 422, e)
		return
	}
	g.SQLiteSource = id
	g.Description = "Vista parcial: hasta 80 triples de la base SQLite. Las consultas acceden a toda la base. El archivo cargado se conserva como copia de solo lectura."
	for _, t := range schema.Tables {
		if len(g.Subgraphs) >= 24 {
			break
		}
		nodes := []string{}
		for i, n := range g.Nodes {
			if strings.HasPrefix(n.ID, t.Class+"/") {
				g.Nodes[i].Group = t.Name
				nodes = append(nodes, n.ID)
			}
		}
		desc := []string{}
		for _, c := range t.Columns {
			desc = append(desc, c.Name)
		}
		g.Subgraphs = append(g.Subgraphs, domain.Subgraph{ID: t.Name, Name: t.Name, Description: "Columnas: " + strings.Join(desc, ", "), Nodes: nodes})
		if len(g.Examples) < 3 && len(t.Columns) > 0 {
			g.Examples = append(g.Examples, domain.Example{Question: "Explorar " + t.Name, Query: "SELECT ?entidad ?valor WHERE {\n  ?entidad <" + t.Columns[0].Predicate + "> ?valor .\n} LIMIT 25"})
		}
	}
	s.store(w, r, &g, 201)
}
