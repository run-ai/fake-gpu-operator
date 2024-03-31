package topology

import (
	"math/rand"
)

func (r *Range) Random() int {
	if r.Min == r.Max {
		return r.Min
	}

	return r.Min + rand.Intn(r.Max-r.Min)
}
