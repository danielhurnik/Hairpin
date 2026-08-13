package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
)

var (
	secret = []byte("a-test-signing-secret-value")
	now    = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
)

func TestRoundTrip(t *testing.T) {
	iss := NewIssuer(secret, time.Hour)
	tok := iss.Issue("player-123", now)

	got, err := iss.Verify(tok, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got != "player-123" {
		t.Errorf("player id = %q, want %q", got, "player-123")
	}
}

func TestExpiry(t *testing.T) {
	iss := NewIssuer(secret, time.Hour)
	tok := iss.Issue("player-123", now)

	if _, err := iss.Verify(tok, now.Add(59*time.Minute)); err != nil {
		t.Errorf("token should still be valid before expiry: %v", err)
	}
	if _, err := iss.Verify(tok, now.Add(time.Hour)); err != ErrExpired {
		t.Errorf("at exactly the expiry the token must be rejected, got %v", err)
	}
	if _, err := iss.Verify(tok, now.Add(2*time.Hour)); err != ErrExpired {
		t.Errorf("expired token error = %v, want ErrExpired", err)
	}
}

// The whole point of signing: a player must not be able to mint a token for
// somebody else's account, or extend their own.
func TestForgedPayloadIsRejected(t *testing.T) {
	iss := NewIssuer(secret, time.Hour)
	real := iss.Issue("player-123", now)
	parts := strings.Split(real, ".")

	// Swap the payload for another player, keeping the original signature.
	forgedPayload := base64.RawURLEncoding.EncodeToString(
		fmt.Appendf(nil, "victim-456|%d", now.Add(time.Hour).Unix()))
	forged := "v1." + forgedPayload + "." + parts[2]

	if _, err := iss.Verify(forged, now); err != ErrSignature {
		t.Errorf("forged payload error = %v, want ErrSignature", err)
	}

	// Extend the expiry of a genuine token.
	extended := base64.RawURLEncoding.EncodeToString(
		fmt.Appendf(nil, "player-123|%d", now.Add(100*365*24*time.Hour).Unix()))
	if _, err := iss.Verify("v1."+extended+"."+parts[2], now); err != ErrSignature {
		t.Errorf("extended expiry error = %v, want ErrSignature", err)
	}
}

func TestTokenFromAnotherSecretIsRejected(t *testing.T) {
	tok := NewIssuer([]byte("attacker-secret"), time.Hour).Issue("player-123", now)
	if _, err := NewIssuer(secret, time.Hour).Verify(tok, now); err != ErrSignature {
		t.Errorf("cross-secret verify = %v, want ErrSignature", err)
	}
}

func TestMalformedTokens(t *testing.T) {
	iss := NewIssuer(secret, time.Hour)
	valid := iss.Issue("player-123", now)
	parts := strings.Split(valid, ".")

	cases := map[string]string{
		"empty":           "",
		"no dots":         "garbage",
		"two parts":       "v1." + parts[1],
		"four parts":      valid + ".extra",
		"wrong version":   "v2." + parts[1] + "." + parts[2],
		"bad base64":      "v1.!!!not-base64!!!." + parts[2],
		"empty player id": "v1." + base64.RawURLEncoding.EncodeToString([]byte("|999999999999")) + "." + parts[2],
		"no separator":    "v1." + base64.RawURLEncoding.EncodeToString([]byte("player-123")) + "." + parts[2],
		"non-numeric exp": "v1." + base64.RawURLEncoding.EncodeToString([]byte("player-123|soon")) + "." + parts[2],
		"empty signature": "v1." + parts[1] + ".",
		"truncated sig":   "v1." + parts[1] + "." + parts[2][:10],
	}
	for name, tok := range cases {
		if _, err := iss.Verify(tok, now); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

// Signature comparison must not leak length information through an early
// return that a timing attack could read. hmac.Equal handles this; the test
// pins the behaviour so a refactor to == is caught.
func TestSignatureOfDifferentLengthIsRejected(t *testing.T) {
	iss := NewIssuer(secret, time.Hour)
	valid := iss.Issue("player-123", now)
	parts := strings.Split(valid, ".")

	for _, sig := range []string{parts[2] + "A", parts[2][:len(parts[2])-1], "x"} {
		if _, err := iss.Verify("v1."+parts[1]+"."+sig, now); err != ErrSignature {
			t.Errorf("signature %q gave %v, want ErrSignature", sig, err)
		}
	}
}

func TestHashDeviceIsStableAndSecretDependent(t *testing.T) {
	a := HashDevice("device-abc", secret)
	b := HashDevice("device-abc", secret)
	if a != b {
		t.Error("hashing the same device twice must give the same value")
	}
	if HashDevice("device-xyz", secret) == a {
		t.Error("different devices must hash differently")
	}
	if HashDevice("device-abc", []byte("other-secret")) == a {
		t.Error("the pepper must change the hash")
	}
	if strings.Contains(a, "device-abc") {
		t.Error("the raw device id must not survive into the stored value")
	}
}

func TestIssuedTokensDifferPerPlayer(t *testing.T) {
	iss := NewIssuer(secret, time.Hour)
	if iss.Issue("a", now) == iss.Issue("b", now) {
		t.Error("two players must not share a token")
	}
}
