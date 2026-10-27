package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/app"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/config"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/domain"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/models"
	"github.com/arturoeanton/go-r2-SPINACH/lab/internal/storage"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "dirección local")
	dbPath := flag.String("db", "data/spinach.db", "archivo SQLite")
	envPath := flag.String("env", ".env", "optional environment file")
	flag.Parse()
	if err := config.LoadEnv(*envPath); err != nil {
		log.Fatal(err)
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatal(err)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		log.Fatal("Esta edición educativa sin autenticación debe escuchar en loopback (127.0.0.1 o ::1).")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		log.Fatal(err)
	}
	repo, err := storage.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer repo.Close()
	for _, seed := range domain.Seeds() {
		_, err = repo.Get(context.Background(), seed.ID)
		if errors.Is(err, domain.ErrNotFound) {
			if err = repo.Save(context.Background(), &seed); err != nil {
				log.Fatal(err)
			}
		} else if err != nil {
			log.Fatal(err)
		}
	}
	services, err := models.FromEnvironment()
	if err != nil {
		log.Fatal(err)
	}

	sources, err := storage.NewSources(filepath.Join(filepath.Dir(*dbPath), "sources"))
	if err != nil {
		log.Fatal(err)
	}
	defer sources.Close()
	server := &http.Server{Addr: *addr, Handler: app.New(repo, services, sources), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 240 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 16}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("SPINACH Lab listo en http://%s", *addr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
