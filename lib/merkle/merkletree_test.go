package merkle

import (
	"hash/adler32"
	"log"
	"math"
	"testing"

	"github.com/si-co/vpir-code/lib/utils"
)

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	rnd := utils.RandomPRG()

	// generate random blocks
	blockLen := 256
	blocks := make([][]byte, 1000)
	for i := range blocks {
		// generate random block
		b := make([]byte, blockLen)
		if _, err := rnd.Read(b); err != nil {
			log.Fatal(err)
		}
		blocks[i] = b
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// generate tree
		_, err := New(blocks)
		if err != nil {
			panic(err)
		}
	}
}

// Code from MerkleTree.go NewUsing() to test mapping from data entry to node index
func TestTreeGen(t *testing.T) {

	// fix key for debugging purposes
	rng := utils.NewPRG(&utils.PRGKey{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	// rng := utils.RandomPRG()

	numRecords := 10000
	data := make([][]byte, numRecords)
	for i := range data {
		d := make([]byte, 32)
		rng.Read(d)
		data[i] = d
	}
	hash := NewBLAKE3()

	branchesLen := int(math.Exp2(math.Ceil(math.Log2(float64(len(data))))))

	// map with the original data to easily loop up the index
	md := make(map[uint32]uint32, len(data))
	// We pad our data length up to the power of 2
	nodes := make([][]byte, branchesLen+len(data)+(branchesLen-len(data)))
	// Leaves
	for i := range data {
		ib := indexToBytes(i)
		nodes[i+branchesLen] = hash.Hash(data[i], ib)

		checksum := adler32.Checksum(data[i])
		if md[checksum] != 0 {
			t.Fatal("collision in checksum output for index ", i)
		}
		md[checksum] = uint32(i)
	}
}
