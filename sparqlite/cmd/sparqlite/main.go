package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/arturoeanton/go-r2-SPINACH/sparqlite"
	"github.com/tggo/goRDFlib/endpoint"
	"github.com/tggo/goRDFlib/graph"
	"github.com/tggo/goRDFlib/store/sqlitestore"
)

func main() {
	source := flag.String("db", "", "SQLite existente para inspección y consultas de solo lectura")
	rdf := flag.String("rdf-store", "", "SQLite para almacenar RDF nativo, incluyendo Update y grafos nombrados")
	write := flag.Bool("write", false, "habilitar escrituras en el almacén RDF nativo")
	addr := flag.String("addr", "127.0.0.1:8091", "dirección loopback")
	base := flag.String("base", sparqlite.DefaultBase, "IRI base del vocabulario generado")
	flag.Parse()
	host, _, e := net.SplitHostPort(*addr)
	if e != nil {
		log.Fatal(e)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		log.Fatal("esta edición local debe escuchar en loopback")
	}
	var handler http.Handler
	if *source != "" && *rdf == "" {
		s, e := sparqlite.Open(*source, *base)
		if e != nil {
			log.Fatal(e)
		}
		defer s.Close()
		handler = s.Handler()
	} else if *rdf != "" && *source == "" {
		st, e := sqlitestore.New(sqlitestore.WithFile(*rdf))
		if e != nil {
			log.Fatal(e)
		}
		defer st.Close()
		opts := []endpoint.Option{endpoint.WithQueryTimeout(20 * time.Second), endpoint.WithUpdateTimeout(20 * time.Second), endpoint.WithMaxSolutions(200000), endpoint.WithMaxResultRows(100000)}
		if !*write {
			opts = append(opts, endpoint.WithReadOnly())
		}
		ep := endpoint.NewForStore(graph.NewDataset(graph.WithStore(st)), opts...)
		mux := http.NewServeMux()
		mux.Handle("/sparql", ep)
		mux.Handle("/data", ep.GraphStoreHandler())
		handler = mux
	} else {
		flag.Usage()
		os.Exit(2)
	}
	server := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("SPARQLite en http://%s/sparql", *addr)
	log.Fatal(server.ListenAndServe())
}
