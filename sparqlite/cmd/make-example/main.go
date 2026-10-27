// make-example creates a disposable relational database for the tutorial.
package main

import (
	"database/sql"
	"flag"
	"log"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

func main() {
	out := flag.String("out", "examples/business.sqlite", "output SQLite file")
	flag.Parse()
	if _, e := os.Stat(*out); e == nil {
		log.Fatal("output already exists; choose a new path")
	}
	if e := os.MkdirAll(filepath.Dir(*out), 0755); e != nil {
		log.Fatal(e)
	}
	db, e := sql.Open("sqlite", *out)
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	_, e = db.Exec(`PRAGMA foreign_keys=ON;
CREATE TABLE customers(id INTEGER PRIMARY KEY, name TEXT NOT NULL, city TEXT);
CREATE TABLE orders(id INTEGER PRIMARY KEY, customer_id INTEGER NOT NULL REFERENCES customers(id), amount REAL NOT NULL);
CREATE INDEX orders_customer ON orders(customer_id);
CREATE INDEX customers_name ON customers(name);
INSERT INTO customers VALUES(1,'Ana','Buenos Aires'),(2,'Sam','London'),(3,'Lina','Paris');
INSERT INTO orders VALUES(101,1,120.5),(102,1,35.0),(103,3,80.0);`)
	if e != nil {
		log.Fatal(e)
	}
	log.Printf("Created %s", *out)
}
