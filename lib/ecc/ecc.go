package ecc

import "errors"

// ECC defines the parameters used for the error correcting code (ECC)
type ECC struct {
	t int
}

func New(t int) *ECC {
	return &ECC{t: t}
}

func (e *ECC) Encode(in uint32) []uint32 {
	out := make([]uint32, e.t+1)
	for i := range out {
		out[i] = in
	}

	return out
}

// Boyer-Moore Majority Vote algorithm
func (e *ECC) Decode(in []uint32) (uint32, error) {
	decoded := in[0]
	votes := 1
	for i := range in[1:] {
		if in[i] == decoded {
			votes++
		} else {
			votes--
		}
	}
	count := 0

	for i := range in {
		if in[i] == decoded {
			count++
		}
	}

	if count > e.t/2 {
		return decoded, nil
	}

	return 0, errors.New("impossible to find a decoded word")
}
