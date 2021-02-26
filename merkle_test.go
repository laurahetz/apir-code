package main

import (
	"fmt"
	"io"
	"testing"

	"github.com/si-co/vpir-code/lib/client"
	"github.com/si-co/vpir-code/lib/database"
	"github.com/si-co/vpir-code/lib/field"
	"github.com/si-co/vpir-code/lib/monitor"
	"github.com/si-co/vpir-code/lib/server"
	"github.com/stretchr/testify/require"
)

func TestMultiBitVectorOneMbMerkle(t *testing.T) {
	dbLen := oneKB
	// we want to download the same numer of bytes
	// as in the field representation
	blockLen := testBlockLength * field.Bytes
	elemBitSize := 8
	nRows := 1
	nCols := dbLen / (elemBitSize * blockLen * nRows)

	// functions defined in vpir_test.go
	xofDB := getXof(t, "db key")
	xof := getXof(t, "client key")

	db := database.CreateRandomMultiBitMerkle(xofDB, dbLen, nRows, blockLen)

	retrieveBlocksDPFMerkle(t, xof, db, nRows*nCols, "DPFMultiBitVectorMerkle")
}

func retrieveBlocksDPFMerkle(t *testing.T, rnd io.Reader, db *database.Bytes, numBlocks int, testName string) {
	c := client.NewPIRdpf(rnd, &db.Info)
	s0 := server.NewPIRdpf(db)
	s1 := server.NewPIRdpf(db)

	totalTimer := monitor.NewMonitor()
	for i := 0; i < numBlocks; i++ {
		fssKeys := c.Query(i, 2)

		a0 := s0.Answer(fssKeys[0])
		a1 := s1.Answer(fssKeys[1])

		answers := [][]byte{a0, a1}

		res, err := c.Reconstruct(answers)
		require.NoError(t, err)
		require.Equal(t, db.Entries[i/db.NumColumns][(i%db.NumColumns)*(db.BlockSize-db.ProofLen):(i%db.NumColumns+1)*(db.BlockSize-db.ProofLen)], res)
	}

	fmt.Printf("TotalCPU time %s: %.1fms\n", testName, totalTimer.Record())
}
