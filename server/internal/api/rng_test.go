package api

import "math/rand/v2"

func newTestRNG() *rand.Rand {
	return rand.New(rand.NewPCG(1, 2))
}
