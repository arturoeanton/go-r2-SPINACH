package storage

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/arturoeanton/go-r2-SPINACH/sparqlite"
)

// Sources owns uploaded relational databases. All paths stay in this directory;
// app handlers never accept arbitrary filesystem paths.
type Sources struct {
	Dir  string
	mu   sync.Mutex
	open map[string]*sparqlite.Source
}

func NewSources(dir string) (*Sources, error) {
	if e := os.MkdirAll(dir, 0755); e != nil {
		return nil, e
	}
	return &Sources{Dir: dir, open: map[string]*sparqlite.Source{}}, nil
}
func (s *Sources) Open(id string) (*sparqlite.Source, error) {
	if raw, e := hex.DecodeString(id); e != nil || len(raw) != 12 {
		return nil, fmt.Errorf("identificador de base inválido")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.open[id]; ok {
		return v, nil
	}
	v, e := sparqlite.Open(filepath.Join(s.Dir, id+".sqlite"), sparqlite.DefaultBase)
	if e != nil {
		return nil, e
	}
	s.open[id] = v
	return v, nil
}
func (s *Sources) Register(ctx context.Context, id string, data []byte) (sparqlite.Schema, error) {
	if len(data) < 16 || string(data[:16]) != "SQLite format 3\x00" {
		return sparqlite.Schema{}, fmt.Errorf("el archivo no es una base SQLite válida")
	}
	if len(data) > 16<<20 {
		return sparqlite.Schema{}, fmt.Errorf("límite de carga: 16 MB; para bases mayores usá el servidor independiente")
	}
	if raw, e := hex.DecodeString(id); e != nil || len(raw) != 12 {
		return sparqlite.Schema{}, fmt.Errorf("identificador inválido")
	}
	path := filepath.Join(s.Dir, id+".sqlite")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return sparqlite.Schema{}, e
	}
	_, e = f.Write(data)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		os.Remove(path)
		return sparqlite.Schema{}, e
	}
	src, e := s.Open(id)
	if e != nil {
		os.Remove(path)
		return sparqlite.Schema{}, e
	}
	schema, e := src.Inspect(ctx)
	return schema, e
}
func (s *Sources) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, src := range s.open {
		_ = src.Close()
	}
	return nil
}
