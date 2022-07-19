package topology

import "math/rand"

func (r *Range) Random() int {
	return r.Min + rand.Intn(r.Max-r.Min)
}
