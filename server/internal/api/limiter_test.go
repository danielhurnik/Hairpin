package api

import (
	"testing"
	"time"
)

func TestLimiterAllowsBurstThenThrottles(t *testing.T) {
	l := newLimiter(5, 10)
	for i := 0; i < 10; i++ {
		if !l.allow("player-a") {
			t.Fatalf("request %d rejected inside the burst allowance", i+1)
		}
	}
	if l.allow("player-a") {
		t.Error("11th request in an instant should be throttled")
	}
	// A different player has their own bucket.
	if !l.allow("player-b") {
		t.Error("throttling one player must not affect another")
	}
}

func TestLimiterRefillsOverTime(t *testing.T) {
	l := newLimiter(1000, 2)
	l.allow("p")
	l.allow("p")
	if l.allow("p") {
		t.Fatal("bucket should be empty")
	}
	time.Sleep(5 * time.Millisecond) // 1000/s refills well past one token
	if !l.allow("p") {
		t.Error("bucket should have refilled")
	}
}

// The floods that matter are the ones aimed at the economy: hammering
// /v1/race/start to farm the ladder faster than the daily allowance intends.
func TestRateLimiterRejectsFloods(t *testing.T) {
	l := newLimiter(5, 20)
	allowed := 0
	for i := 0; i < 200; i++ {
		if l.allow("flooder") {
			allowed++
		}
	}
	if allowed > 21 {
		t.Errorf("limiter let %d/200 instant requests through, want ~20", allowed)
	}
	if allowed < 20 {
		t.Errorf("limiter allowed only %d, the configured burst is 20", allowed)
	}
}
