// Package auth issues and verifies session tokens for anonymous device
// accounts. Tokens are HMAC-SHA256 signed and carry only a player ID and an
// expiry — there is no client-supplied state the server trusts.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMalformed = errors.New("malformed token")
	ErrSignature = errors.New("bad token signature")
	ErrExpired   = errors.New("token expired")
)

type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func NewIssuer(secret []byte, ttl time.Duration) *Issuer {
	return &Issuer{secret: secret, ttl: ttl}
}

// Issue returns a token of the form v1.<payload>.<sig>.
func (i *Issuer) Issue(playerID string, now time.Time) string {
	payload := fmt.Sprintf("%s|%d", playerID, now.Add(i.ttl).Unix())
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return "v1." + enc + "." + i.sign(enc)
}

// Verify returns the player ID carried by a valid, unexpired token.
func (i *Issuer) Verify(token string, now time.Time) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", ErrMalformed
	}
	if !hmac.Equal([]byte(i.sign(parts[1])), []byte(parts[2])) {
		return "", ErrSignature
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrMalformed
	}
	pid, expStr, ok := strings.Cut(string(raw), "|")
	if !ok || pid == "" {
		return "", ErrMalformed
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", ErrMalformed
	}
	if now.Unix() >= exp {
		return "", ErrExpired
	}
	return pid, nil
}

func (i *Issuer) sign(payload string) string {
	m := hmac.New(sha256.New, i.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// HashDevice turns a client device secret into the value we store. We never
// persist the raw device ID, so a database leak does not hand out sessions.
func HashDevice(deviceID string, pepper []byte) string {
	m := hmac.New(sha256.New, pepper)
	m.Write([]byte(deviceID))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
