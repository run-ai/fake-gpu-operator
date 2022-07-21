package topology

import (
	"math/rand"
	"time"
)

func (r *Range) Random() int {
	if r.Min == r.Max {
		return r.Min
	}

	rand.Seed(time.Now().UnixNano())
	return r.Min + rand.Intn(r.Max-r.Min)
}
