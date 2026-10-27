// Package sparqlite exposes existing SQLite databases as virtual RDF graphs.
// Source files are opened read-only. No tables, indexes or RDF copies are added.
package sparqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tggo/goRDFlib/term"
	"modernc.org/sqlite"
)

const DefaultBase = "https://sqlite.local/"

type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	PrimaryKey int    `json:"primaryKey"`
	Nullable   bool   `json:"nullable"`
	Predicate  string `json:"predicate"`
}
type ForeignKey struct {
	ID        int      `json:"id"`
	Table     string   `json:"table"`
	From      []string `json:"from"`
	To        []string `json:"to"`
	Predicate string   `json:"predicate"`
}
type Index struct {
	Name   string `json:"name"`
	Unique bool   `json:"unique"`
}
type Table struct {
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Columns      []Column     `json:"columns"`
	ForeignKeys  []ForeignKey `json:"foreignKeys"`
	Indexes      []Index      `json:"indexes"`
	Class        string       `json:"class"`
	Identity     string       `json:"identity"`
	pk           []string
	rowid        string
	withoutRowid bool
}
type Schema struct {
	Base     string   `json:"base"`
	Version  int      `json:"version"`
	Tables   []Table  `json:"tables"`
	Warnings []string `json:"warnings"`
}
type Source struct {
	db     *sql.DB
	Base   string
	mu     sync.Mutex
	cached *Schema
}
type relation struct {
	sql, predicate, table string
	subject               string
	from                  string
	where                 string
	object                string
	identity              *Table
	column                string
	objectIdentity        *Table
}

func qi(s string) string     { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func lit(s string) string    { return `'` + strings.ReplaceAll(s, `'`, `''`) + `'` }
func iriKey(s string) string { return term.TermKey(term.NewURIRefUnsafe(s)) }

var register sync.Once

func registerFunctions() {
	register.Do(func() {
		sqlite.MustRegisterDeterministicScalarFunction("sparqlite_term", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if args[0] == nil {
				return nil, nil
			}
			var t term.Term
			switch v := args[0].(type) {
			case []byte:
				t = term.NewLiteral(strings.ToUpper(hex.EncodeToString(v)), term.WithDatatype(term.NewURIRefUnsafe(term.XSDNamespace+"hexBinary")))
			default:
				t = term.NewLiteral(v)
			}
			return term.TermKey(t), nil
		})
		sqlite.MustRegisterDeterministicScalarFunction("sparqlite_subject", -1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("identidad inválida")
			}
			base, _ := args[0].(string)
			table, _ := args[1].(string)
			mode, _ := args[2].(string)
			parts := []string{}
			for i := 3; i < len(args); i += 2 {
				if i+1 >= len(args) || args[i+1] == nil {
					return nil, nil
				}
				parts = append(parts, url.PathEscape(fmt.Sprint(args[i]))+"="+url.PathEscape(fmt.Sprint(args[i+1])))
			}
			value := base + url.PathEscape(table) + "/" + strings.Join(parts, ";")
			if mode == "blank" {
				return term.TermKey(term.NewBNode(hex.EncodeToString([]byte(value)))), nil
			}
			return iriKey(value), nil
		})
	})
}

func Open(path, base string) (*Source, error) {
	registerFunctions()
	if base == "" {
		base = DefaultBase
	}
	u, e := url.Parse(base)
	if e != nil || !u.IsAbs() {
		return nil, fmt.Errorf("base IRI inválida")
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	abs, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String() + "?mode=ro&_pragma=busy_timeout(5000)"
	db, e := sql.Open("sqlite", dsn)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(8)
	if e = db.Ping(); e != nil {
		db.Close()
		return nil, e
	}
	s := &Source{db: db, Base: base}
	if _, e = s.Inspect(context.Background()); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Source) Close() error { return s.db.Close() }

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Source) Inspect(ctx context.Context) (Schema, error) {
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return Schema{}, e
	}
	defer tx.Rollback()
	return s.schema(ctx, tx)
}
func (s *Source) schema(ctx context.Context, db querier) (Schema, error) {
	var version int
	if e := db.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&version); e != nil {
		return Schema{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && s.cached.Version == version {
		return *s.cached, nil
	}
	v, e := inspect(ctx, db, s.Base)
	if e == nil {
		s.cached = &v
	}
	return v, e
}
func inspect(ctx context.Context, db querier, base string) (Schema, error) {
	schema := Schema{Base: base, Tables: []Table{}, Warnings: []string{}}
	if err := db.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&schema.Version); err != nil {
		return schema, err
	}
	rows, e := db.QueryContext(ctx, `SELECT name,type FROM sqlite_schema WHERE type IN ('table','view') AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	if e != nil {
		return schema, e
	}
	for rows.Next() {
		var t Table
		if e = rows.Scan(&t.Name, &t.Kind); e != nil {
			rows.Close()
			return schema, e
		}
		t.Class = base + url.PathEscape(t.Name)
		schema.Tables = append(schema.Tables, t)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return schema, e
	}
	if len(schema.Tables) > 300 {
		return schema, fmt.Errorf("más de 300 tablas: usá una vista o una base acotada")
	}
	for i := range schema.Tables {
		t := &schema.Tables[i]
		var wr int
		if err := db.QueryRowContext(ctx, "SELECT wr FROM pragma_table_list WHERE schema='main' AND name=?", t.Name).Scan(&wr); err != nil {
			return schema, err
		}
		t.withoutRowid = wr != 0
		t.Columns = []Column{}
		t.ForeignKeys = []ForeignKey{}
		t.Indexes = []Index{}
		rows, e = db.QueryContext(ctx, "SELECT name,type,\"notnull\",pk,hidden FROM pragma_table_xinfo(?)", t.Name)
		if e != nil {
			return schema, e
		}
		for rows.Next() {
			var c Column
			var notnull, hidden int
			if e = rows.Scan(&c.Name, &c.Type, &notnull, &c.PrimaryKey, &hidden); e != nil {
				rows.Close()
				return schema, e
			}
			if hidden == 1 {
				continue
			}
			c.Nullable = notnull == 0 && c.PrimaryKey == 0
			c.Predicate = t.Class + "#" + url.PathEscape(c.Name)
			t.Columns = append(t.Columns, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return schema, e
		}
		keys := []Column{}
		for _, c := range t.Columns {
			if c.PrimaryKey > 0 {
				keys = append(keys, c)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].PrimaryKey < keys[j].PrimaryKey })
		for _, c := range keys {
			t.pk = append(t.pk, c.Name)
		}
		for _, alias := range []string{"_rowid_", "rowid", "oid"} {
			shadow := false
			for _, c := range t.Columns {
				if strings.EqualFold(c.Name, alias) {
					shadow = true
				}
			}
			if !shadow && t.Kind != "view" && !t.withoutRowid {
				t.rowid = alias
				break
			}
		}
		if len(t.pk) > 0 {
			t.Identity = "primary-key"
		} else {
			t.Identity = "blank-node-rowid"
			if t.rowid == "" {
				t.Identity = "blank-node-row-number"
				schema.Warnings = append(schema.Warnings, t.Name+": sin clave primaria ni rowid accesible; identidad válida dentro de la consulta, no estable entre cambios.")
			}
		}
		rows, e = db.QueryContext(ctx, "SELECT id,seq,\"table\",\"from\",\"to\" FROM pragma_foreign_key_list(?) ORDER BY id,seq", t.Name)
		if e != nil {
			return schema, e
		}
		fks := map[int]*ForeignKey{}
		order := []int{}
		for rows.Next() {
			var id, seq int
			var target, from string
			var to sql.NullString
			if e = rows.Scan(&id, &seq, &target, &from, &to); e != nil {
				rows.Close()
				return schema, e
			}
			f := fks[id]
			if f == nil {
				f = &ForeignKey{ID: id, Table: target}
				fks[id] = f
				order = append(order, id)
			}
			f.From = append(f.From, from)
			f.To = append(f.To, to.String)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return schema, e
		}
		for _, id := range order {
			f := *fks[id]
			f.Predicate = t.Class + "#ref-" + url.PathEscape(strings.Join(f.From, ";"))
			t.ForeignKeys = append(t.ForeignKeys, f)
		}
		rows, e = db.QueryContext(ctx, "SELECT name,\"unique\" FROM pragma_index_list(?)", t.Name)
		if e != nil {
			return schema, e
		}
		for rows.Next() {
			var ix Index
			var unique int
			if e = rows.Scan(&ix.Name, &unique); e != nil {
				rows.Close()
				return schema, e
			}
			ix.Unique = unique != 0
			t.Indexes = append(t.Indexes, ix)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return schema, e
		}
		if len(t.ForeignKeys) == 0 {
			schema.Warnings = append(schema.Warnings, t.Name+": sin claves foráneas declaradas; no se infieren relaciones por similitud de nombres.")
		}
	}
	return schema, nil
}
func subjectSQL(base string, t *Table, alias string) string {
	args := []string{lit(base), lit(t.Name)}
	if len(t.pk) > 0 {
		args = append(args, "'iri'")
		for _, key := range t.pk {
			args = append(args, lit(key), alias+"."+qi(key))
		}
	} else {
		args = append(args, "'blank'", "'row'", alias+"."+qi(t.rowid))
		if t.rowid == "" {
			args[len(args)-1] = alias + ".\"__sparqlite_row\""
		}
	}
	expr := "sparqlite_subject(" + strings.Join(args, ",") + ")"
	if len(t.pk) > 0 && !t.withoutRowid {
		fallback := t.rowid
		if fallback == "" {
			fallback = "__sparqlite_row"
		}
		return "coalesce(" + expr + ",sparqlite_subject(" + lit(base) + "," + lit(t.Name) + ",'blank','row'," + alias + "." + qi(fallback) + "))"
	}
	return expr
}
func tableSQL(t *Table, alias string) string {
	if t.rowid == "" && !t.withoutRowid {
		return "(SELECT *, row_number() OVER () AS \"__sparqlite_row\" FROM " + qi(t.Name) + ") " + alias
	}
	return qi(t.Name) + " " + alias
}
func relations(schema Schema) []relation {
	out := []relation{}
	byName := map[string]*Table{}
	for i := range schema.Tables {
		byName[schema.Tables[i].Name] = &schema.Tables[i]
	}
	for i := range schema.Tables {
		t := &schema.Tables[i]
		subject := subjectSQL(schema.Base, t, "r")
		from := tableSQL(t, "r")
		valid := "1"
		add := func(pred, obj, fr, where string) {
			p := iriKey(pred)
			out = append(out, relation{sql: "SELECT " + subject + " AS s," + lit(p) + " AS p," + obj + " AS o FROM " + fr + " WHERE " + where, predicate: pred, table: t.Name, subject: subject, from: fr, where: where, object: obj, identity: t})
		}
		add(term.RDFNamespace+"type", lit(iriKey(t.Class)), from, valid)
		for _, c := range t.Columns {
			col := "r." + qi(c.Name)
			add(c.Predicate, "sparqlite_term("+col+")", from, valid+" AND "+col+" IS NOT NULL")
			out[len(out)-1].column = c.Name
		}
		for _, f := range t.ForeignKeys {
			target := byName[f.Table]
			if target == nil {
				continue
			}
			ons := []string{}
			for j, col := range f.From {
				to := f.To[j]
				if to == "" && j < len(target.pk) {
					to = target.pk[j]
				}
				if to == "" {
					ons = nil
					break
				}
				ons = append(ons, "r."+qi(col)+"=t."+qi(to))
			}
			if len(ons) == 0 {
				continue
			}
			object := subjectSQL(schema.Base, target, "t")
			add(f.Predicate, object, from+" JOIN "+tableSQL(target, "t")+" ON "+strings.Join(ons, " AND "), valid)
			out[len(out)-1].objectIdentity = target
		}
	}
	return out
}
