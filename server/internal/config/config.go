package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr string
	DSN  string
	// TokenSecret signs session tokens. MUST be set in production; a random
	// one is generated in dev, which invalidates all sessions on restart.
	TokenSecret []byte
	TokenTTL    time.Duration
	// TimeScale divides every job timer. 1 = real time. Set to e.g. 60 in dev
	// so a four-hour job finishes in four minutes.
	TimeScale float64
	// DevIAP lets /v1/store/purchase grant rubies without a real receipt.
	// Never enable this in production.
	DevIAP bool
	// SeedBots is how many synthetic ladder rivals to create on first boot.
	SeedBots int
	// Per-player token bucket over mutating requests.
	RatePerSec float64
	RateBurst  float64
}

func Load() Config {
	c := Config{
		Addr:       env("HAIRPIN_ADDR", ":8080"),
		DSN:        env("HAIRPIN_DSN", "file:hairpin.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"),
		TokenTTL:   30 * 24 * time.Hour,
		TimeScale:  envFloat("HAIRPIN_TIME_SCALE", 1),
		DevIAP:     env("HAIRPIN_DEV_IAP", "true") == "true",
		SeedBots:   int(envFloat("HAIRPIN_SEED_BOTS", 120)),
		RatePerSec: envFloat("HAIRPIN_RATE_PER_SEC", 5),
		RateBurst:  envFloat("HAIRPIN_RATE_BURST", 20),
	}

	if s := os.Getenv("HAIRPIN_TOKEN_SECRET"); s != "" {
		c.TokenSecret = []byte(s)
	} else {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("generating dev token secret: %v", err)
		}
		c.TokenSecret = []byte(hex.EncodeToString(b))
		log.Println("WARNING: HAIRPIN_TOKEN_SECRET unset — using a random dev secret; all sessions drop on restart")
	}
	if c.TimeScale <= 0 {
		c.TimeScale = 1
	}
	if c.RatePerSec <= 0 {
		c.RatePerSec = 5
	}
	if c.RateBurst <= 0 {
		c.RateBurst = 20
	}
	return c
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
