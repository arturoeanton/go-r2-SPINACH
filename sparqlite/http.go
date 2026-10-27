package sparqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tggo/goRDFlib/sparql"
	"github.com/tggo/goRDFlib/sparql/results"
	"github.com/tggo/goRDFlib/turtle"
)

// Handler serves read-only SPARQL Protocol queries against the virtual mapping.
// /schema exposes the vocabulary so clients can discover it without guessing.
func (s *Source) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /schema", func(w http.ResponseWriter, r *http.Request) {
		v, e := s.Inspect(r.Context())
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	})
	mux.HandleFunc("/sparql", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		query := ""
		switch r.Method {
		case "GET":
			query = r.URL.Query().Get("query")
		case "POST":
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			switch strings.Split(r.Header.Get("Content-Type"), ";")[0] {
			case "application/sparql-query":
				raw, e := io.ReadAll(r.Body)
				if e != nil {
					http.Error(w, e.Error(), 413)
					return
				}
				query = string(raw)
			case "application/x-www-form-urlencoded":
				if e := r.ParseForm(); e != nil {
					http.Error(w, e.Error(), 400)
					return
				}
				if r.Form.Has("update") {
					http.Error(w, "SPARQL Update no está habilitado sobre tablas relacionales", 403)
					return
				}
				query = r.Form.Get("query")
			case "application/sparql-update":
				http.Error(w, "SPARQL Update no está habilitado sobre tablas relacionales", 403)
				return
			default:
				http.Error(w, "se requiere application/sparql-query o application/x-www-form-urlencoded", 415)
				return
			}
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "método no permitido", 405)
			return
		}
		if strings.TrimSpace(query) == "" {
			http.Error(w, "falta query", 400)
			return
		}
		if r.URL.Query().Has("default-graph-uri") || r.URL.Query().Has("named-graph-uri") || r.Form.Has("default-graph-uri") || r.Form.Has("named-graph-uri") {
			http.Error(w, "usá FROM/FROM NAMED; los parámetros HTTP de dataset aún no están implementados en el mapeo virtual", 422)
			return
		}
		result, report, e := s.Query(ctx, query)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		w.Header().Set("X-SPARQLite-Engine", report.Engine)
		w.Header().Set("Server-Timing", fmt.Sprintf("query;dur=%.3f", report.DurationMS))
		if e = WriteResult(w, r, result); e != nil {
			http.Error(w, e.Error(), 406)
		}
	})
	return mux
}
func WriteResult(w http.ResponseWriter, r *http.Request, result *sparql.Result) error {
	var out bytes.Buffer
	contentType := "text/turtle; charset=utf-8"
	if result.Graph != nil {
		if e := turtle.Serialize(r.Context(), result.Graph, &out); e != nil {
			return e
		}
	} else {
		accept := r.Header.Get("Accept")
		if accept == "" {
			accept = "application/sparql-results+json"
		}
		format, ok := results.Negotiate(accept)
		if !ok {
			return fmt.Errorf("formato de resultados no admitido")
		}
		contentType = format.ContentType()
		if e := results.Write(&out, format, result); e != nil {
			return e
		}
	}
	w.Header().Set("Content-Type", contentType)
	_, e := w.Write(out.Bytes())
	return e
}
