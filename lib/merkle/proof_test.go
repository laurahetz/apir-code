package merkle

import (
	"testing"

	"github.com/si-co/vpir-code/lib/utils"
	"github.com/stretchr/testify/require"
)

// func TestEncodeDecodeProof(t *testing.T) {
// 	rng := utils.RandomPRG()
// 	data := make([][]byte, rand.Intn(501))
// 	for i := range data {
// 		d := make([]byte, 32)
// 		rng.Read(d)
// 		data[i] = d
// 	}

// 	// create the tree
// 	tree, err := New(data)
// 	require.NoError(t, err)

// 	// generate a proof for random element
// 	proof, err := tree.GenerateProof(data[rand.Intn(len(data))])
// 	require.NoError(t, err)

// 	// encode the proof
// 	b := EncodeProof(proof)

// 	// decode proof
// 	p := DecodeProof(b)

// 	require.Equal(t, *proof, *p)
// }

// Test Proof encoding, decoding and verification of each data item
func TestProofVerification(t *testing.T) {
	// fix key for debugging purposes
	rng := utils.NewPRG(&utils.PRGKey{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	//rng := utils.RandomPRG()

	numRecords := 10000
	data := make([][]byte, numRecords)
	for i := range data {
		d := make([]byte, 32)
		rng.Read(d)
		data[i] = d
	}

	// create the tree
	tree, err := New(data)
	require.NoError(t, err)

	// generate a proof for EVERY element
	for i := range data {
		proof, err := tree.GenerateProof(data[i])
		require.NoError(t, err)

		// Check Encoding for each element
		// encode the proof
		b := EncodeProof(proof)
		// decode proof
		p := DecodeProof(b)
		require.Equal(t, *proof, *p)

		// check if proof verifies
		vrf, err := VerifyProof(data[i], proof, tree.Root())
		require.NoError(t, err)
		if !vrf {
			t.Fatal("Proof with index ", i, " did not verify")
		}
	}
}
