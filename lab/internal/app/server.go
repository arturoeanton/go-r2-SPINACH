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
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
	Delete(string) error
}
type Server struct {
	Repo      domain.Repository
	Sources   SourceRegistry
	Decisions models.DecisionModel
	Language  models.LanguageModel
	Models    *models.Runtime
	Mux       *http.ServeMux
	graphMu   sync.Mutex // Serialize graph writes and last-reference source cleanup.
}

func New(repo domain.Repository, services *models.Runtime, sources SourceRegistry) *Server {
	s := &Server{Repo: repo, Sources: sources, Decisions: services, Language: services, Models: services, Mux: http.NewServeMux()}
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
	s.Mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, s.Models.PublicConfig()) })
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
func (s *Server) store(w http.ResponseWriter, r *http.Request, g *domain.Graph, status int) bool {
	s.graphMu.Lock()
	defer s.graphMu.Unlock()
	if err := g.Validate(); err != nil {
		fail(w, 422, err)
		return false
	}
	if g.SQLiteSource != "" {
		if _, err := s.Sources.Open(g.SQLiteSource); err != nil {
			fail(w, 422, errors.New("la base SQLite ya no está disponible; volvé a importarla"))
			return false
		}
	}
	if err := s.Repo.Save(r.Context(), g); err != nil {
		repoError(w, err)
		return false
	}
	reply(w, status, g)
	return true
}
func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	s.graphMu.Lock()
	defer s.graphMu.Unlock()
	g, ok := s.graph(w, r)
	if !ok {
		return
	}
	removeSource := g.SQLiteSource != ""
	if removeSource {
		graphs, err := s.Repo.List(r.Context())
		if err != nil {
			repoError(w, err)
			return
		}
		for _, other := range graphs {
			if other.ID != g.ID && other.SQLiteSource == g.SQLiteSource {
				removeSource = false
				break
			}
		}
	}
	if err := s.Repo.Delete(r.Context(), g.ID); err != nil {
		repoError(w, err)
		return
	}
	if removeSource {
		if err := s.Sources.Delete(g.SQLiteSource); err != nil {
			// Retain a visible, retryable graph if filesystem cleanup fails.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			g.Revision = 0
			if restoreErr := s.Repo.Save(ctx, &g); restoreErr != nil {
				log.Printf("restore graph after SQLite removal failure: %v", restoreErr)
			}
			fail(w, 500, errors.New("no se pudo eliminar la copia SQLite; recargá e intentá nuevamente"))
			return
		}
	}
	reply(w, 200, map[string]bool{"deleted": true, "sourceDeleted": removeSource})
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
	Question      string         `json:"question"`
	Focus         string         `json:"focus"`
	Language      string         `json:"language,omitempty"`
	UseNavigation *bool          `json:"useNavigation,omitempty"`
	MeasureTime   bool           `json:"measureTime,omitempty"`
	Events        []models.Event `json:"events"`
}

func (b request) usesNavigation() bool {
	return b.UseNavigation == nil || *b.UseNavigation
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
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
	d := models.Decision{Source: "disabled", Candidates: []models.Candidate{}}
	var err error
	navigationStart := time.Now()
	if b.usesNavigation() {
		_, navigationModel := s.Models.Models()
		d, err = s.Decisions.Rank(r.Context(), g, b.Question, navigationModel)
	}
	if err != nil {
		s.failStep(w, err, b.MeasureTime, start, models.Timing{NavigationMS: elapsedMS(navigationStart)}, &models.Event{Navigation: &d})
		return
	}
	if b.MeasureTime {
		d.Timing = &models.Timing{TotalMS: elapsedMS(start)}
		if b.usesNavigation() {
			d.Timing.NavigationMS = elapsedMS(navigationStart)
		}
	}
	reply(w, 200, d)
}
func validRequest(w http.ResponseWriter, b request) bool {
	if strings.TrimSpace(b.Question) == "" || len(b.Question) > 2000 || len(b.Events) > 8 {
		fail(w, 422, errors.New("se requiere una pregunta válida; máximo 8 pasos"))
		return false
	}
	return true
}
func (s *Server) step(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
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
	timing := models.Timing{}
	replayStart := time.Now()
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
	timing.ReplayMS = elapsedMS(replayStart)
	model, navigationModel := s.Models.Models()
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
	var navigation *models.Decision
	focus := ""
	if b.usesNavigation() {
		navigationStart := time.Now()
		d, err := s.Decisions.Rank(r.Context(), g, navigationState, navigationModel)
		timing.NavigationMS = elapsedMS(navigationStart)
		if err != nil {
			s.failStep(w, err, b.MeasureTime, start, timing, &models.Event{Navigation: &d})
			return
		}
		navigation = &d
		focus = d.Choice
	}
	generationStart := time.Now()
	a, err := s.Language.Next(r.Context(), g, b.Question, model, focus, b.Language, events)
	timing.GenerationMS = elapsedMS(generationStart)
	if err != nil {
		s.failStep(w, err, b.MeasureTime, start, timing, &models.Event{Navigation: navigation, Generation: s.Models.Trace(model, a)})
		return
	}
	var event models.Event
	toolStart := time.Now()
	if a.Type == "answer" && !hasQuery {
		event = models.Event{Action: a, Error: "Antes de responder, ejecutá una consulta exitosa para obtener evidencia."}
	} else {
		event = s.execute(r.Context(), g, a)
	}
	timing.ToolMS = elapsedMS(toolStart)
	event.Navigation = navigation
	event.Generation = s.Models.Trace(model, a)
	if b.MeasureTime {
		timing.TotalMS = elapsedMS(start)
		event.Timing = &timing
	}
	reply(w, 200, event)
}

func (s *Server) failStep(w http.ResponseWriter, err error, measure bool, start time.Time, timing models.Timing, trace *models.Event) {
	body := map[string]any{"error": err.Error(), "trace": trace}
	if measure {
		timing.TotalMS = elapsedMS(start)
		body["timing"] = timing
	}
	reply(w, 502, body)
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
		ev.Observation = map[string]string{"answer": a.Answer, "notice": "Síntesis generada por el modelo; verificá su correspondencia con la tabla de evidencia."}
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
	go func() { ch <- result{"generation", s.Models.LanguageStatus(r.Context())} }()
	go func() { ch <- result{"navigation", s.Models.NavigationStatus(r.Context())} }()
	out := s.Models.PublicConfig()
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
	saved := false
	defer func() {
		if !saved {
			if err := s.Sources.Delete(id); err != nil {
				log.Printf("clean up failed SQLite import: %v", err)
			}
		}
	}()
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
	saved = s.store(w, r, &g, 201)
}
