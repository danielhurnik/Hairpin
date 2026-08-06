// Command hairpin runs the Hairpin game server.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/api"
	"github.com/danielhurnik/hairpin/server/internal/config"
	"github.com/danielhurnik/hairpin/server/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("hairpin ")

	cfg := config.Load()

	st, err := store.Open(cfg.DSN, store.Options{TimeScale: cfg.TimeScale})
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	// The ladder needs opponents before it has players.
	added, err := st.SeedBots(context.Background(), cfg.SeedBots)
	if err != nil {
		log.Fatalf("seed bots: %v", err)
	}
	if added > 0 {
		log.Printf("seeded %d ladder rivals", added)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.New(cfg, st).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		log.Printf("listening on %s (time scale %.0fx)", cfg.Addr, cfg.TimeScale)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
