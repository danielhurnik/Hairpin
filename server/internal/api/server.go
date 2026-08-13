// Package api exposes the game over HTTP. Handlers parse arguments and render
// responses; every rule lives in internal/store and internal/game. If a
// handler ever computes a currency amount or a race outcome, that is a bug.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielhurnik/hairpin/server/internal/auth"
	"github.com/danielhurnik/hairpin/server/internal/config"
	"github.com/danielhurnik/hairpin/server/internal/store"
)

type Server struct {
	cfg    config.Config
	store  *store.Store
	issuer *auth.Issuer
	limit  *limiter
}

func New(cfg config.Config, st *store.Store) *Server {
	return &Server{
		cfg:    cfg,
		store:  st,
		issuer: auth.NewIssuer(cfg.TokenSecret, cfg.TokenTTL),
		limit:  newLimiter(cfg.RatePerSec, cfg.RateBurst),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/content", s.content)
	mux.HandleFunc("POST /v1/auth/device", s.authDevice)

	// Everything below requires a session token.
	auth := func(pattern string, h authedHandler) {
		mux.Handle(pattern, s.authed(h))
	}

	auth("GET /v1/state", s.state)
	auth("GET /v1/me", s.me)
	auth("POST /v1/me/name", s.setName)

	auth("GET /v1/jobs", s.jobs)
	auth("POST /v1/jobs/start", s.startJob)
	auth("POST /v1/jobs/claim", s.claimJob)
	auth("POST /v1/jobs/rush", s.rushJob)

	auth("GET /v1/garage", s.garage)
	auth("POST /v1/garage/upgrade", s.buyUpgrade)
	auth("POST /v1/driver/allocate", s.allocate)

	auth("GET /v1/cars", s.cars)
	auth("POST /v1/cars/buy", s.buyCar)
	auth("POST /v1/cars/equip", s.equipCar)

	auth("GET /v1/setup", s.setup)
	auth("POST /v1/setup", s.setSetup)
	auth("POST /v1/setup/reset", s.resetSetup)

	auth("GET /v1/inventory", s.inventory)
	auth("POST /v1/inventory/equip", s.equip)
	auth("POST /v1/inventory/unequip", s.unequip)
	auth("POST /v1/inventory/sell", s.sell)

	auth("GET /v1/race/opponent", s.opponent)
	auth("POST /v1/race/start", s.race)
	auth("GET /v1/race/history", s.history)
	auth("GET /v1/race/replay/{id}", s.replay)

	auth("GET /v1/ladder", s.ladder)

	auth("GET /v1/shop", s.shop)
	auth("POST /v1/shop/buy", s.shopBuy)
	auth("GET /v1/liveries", s.liveries)
	auth("POST /v1/liveries/buy", s.buyLivery)
	auth("POST /v1/liveries/equip", s.equipLivery)
	auth("POST /v1/store/purchase", s.purchaseRubies)

	return logging(cors(mux))
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

type ctxKey int

const playerKey ctxKey = 1

type authedHandler func(http.ResponseWriter, *http.Request, *store.Player)

func (s *Server) authed(h authedHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			fail(w, http.StatusUnauthorized, "missing_token", "no bearer token")
			return
		}
		pid, err := s.issuer.Verify(token, time.Now())
		if err != nil {
			fail(w, http.StatusUnauthorized, "bad_token", err.Error())
			return
		}
		if r.Method != http.MethodGet && !s.limit.allow(pid) {
			fail(w, http.StatusTooManyRequests, "rate_limited", "slow down")
			return
		}
		p, err := s.store.GetPlayer(r.Context(), pid)
		if err != nil {
			writeErr(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), playerKey, p)
		h(w, r.WithContext(ctx), p)
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) {
	w.code = c
	w.ResponseWriter.WriteHeader(c)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
	})
}

// limiter is a small per-player token bucket over mutating requests. It is not
// a substitute for the server-side rules — it just makes brute-forcing the
// endpoints expensive.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64
	burst   float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newLimiter(rate, burst float64) *limiter {
	if rate <= 0 {
		rate = 5
	}
	if burst <= 0 {
		burst = 20
	}
	l := &limiter{buckets: map[string]*bucket{}, rate: rate, burst: burst}
	go l.sweep()
	return l
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *limiter) sweep() {
	for range time.Tick(10 * time.Minute) {
		l.mu.Lock()
		cutoff := time.Now().Add(-30 * time.Minute)
		for k, b := range l.buckets {
			if b.last.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// ---------------------------------------------------------------------------
// Responses
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

type errBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func fail(w http.ResponseWriter, code int, kind, msg string) {
	writeJSON(w, code, errBody{Error: kind, Message: msg})
}

// writeErr maps domain errors onto status codes in one place, so handlers can
// simply propagate.
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, store.ErrInsufficient):
		fail(w, http.StatusPaymentRequired, "insufficient_funds", "you cannot afford that")
	case errors.Is(err, store.ErrInvalid):
		fail(w, http.StatusBadRequest, "invalid", "invalid request")
	case errors.Is(err, store.ErrJobActive):
		fail(w, http.StatusConflict, "job_active", "a job is already running")
	case errors.Is(err, store.ErrJobNotDone):
		fail(w, http.StatusConflict, "job_not_done", "that job is not finished yet")
	case errors.Is(err, store.ErrNoJob):
		fail(w, http.StatusConflict, "no_job", "no job is running")
	case errors.Is(err, store.ErrNoRaces):
		fail(w, http.StatusConflict, "no_races", "no races left today")
	case errors.Is(err, store.ErrMaxTier):
		fail(w, http.StatusConflict, "max_tier", "already fully upgraded")
	case errors.Is(err, store.ErrNoOpponent):
		fail(w, http.StatusConflict, "no_opponent", "nobody to race")
	case errors.Is(err, store.ErrAlreadyOwned):
		fail(w, http.StatusConflict, "already_owned", "already owned")
	case errors.Is(err, store.ErrLevelLocked):
		fail(w, http.StatusConflict, "level_locked", "your driver level is too low for that")
	case errors.Is(err, store.ErrPartMissing):
		fail(w, http.StatusConflict, "part_missing", "this car has nothing to adjust there yet")
	case errors.Is(err, store.ErrOfferPurchased):
		fail(w, http.StatusConflict, "offer_taken", "you already bought that")
	default:
		log.Printf("internal error: %v", err)
		fail(w, http.StatusInternalServerError, "internal", "something went wrong")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "bad_body", err.Error())
		return false
	}
	return true
}

func intQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
