package sparqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestWithoutRowidAndNullableKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.sqlite")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`CREATE TABLE composite(a TEXT,b INTEGER,PRIMARY KEY(a,b)) WITHOUT ROWID;
 INSERT INTO composite VALUES('x',1),('x',2);
 CREATE TABLE nullable(a TEXT,b INTEGER,PRIMARY KEY(a,b));
 INSERT INTO nullable VALUES(NULL,1),(NULL,1),('x',1);
 CREATE VIEW projected AS SELECT a,b FROM nullable;`)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(path, "")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for table, want := range map[string]int{"composite": 2, "nullable": 3, "projected": 3} {
		q := `SELECT ?s WHERE { ?s a <https://sqlite.local/` + table + `> }`
		result, _, e := s.Query(context.Background(), q)
		if e != nil {
			t.Fatalf("%s: %v", table, e)
		}
		if len(result.Bindings) != want {
			t.Fatalf("%s: %d, want %d", table, len(result.Bindings), want)
		}
	}
}
