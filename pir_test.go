package main

import (
	"fmt"
	"io"
	"math"
	"testing"

	"github.com/si-co/vpir-code/lib/client"
	"github.com/si-co/vpir-code/lib/database"
	"github.com/si-co/vpir-code/lib/field"
	"github.com/si-co/vpir-code/lib/monitor"
	"github.com/si-co/vpir-code/lib/server"
	"github.com/stretchr/testify/require"
)

func TestMultiBitVectorOneMbPIR(t *testing.T) {
	dbLen := oneMB
	// we want to download the same numer of bytes
	// as in the field representation
	blockLen := testBlockLength * field.Bytes
	elemBitSize := 8
	nRows := 1
	nCols := dbLen / (elemBitSize * blockLen * nRows)

	// functions defined in vpir_test.go
	xofDB := getXof(t, "db key")
	xof := getXof(t, "client key")

	db := database.CreateRandomMultiBitBytes(xofDB, dbLen, nRows, blockLen)

	retrieveBlocksBytes(t, xof, db, nRows*nCols, "MultiBitVectorOneMbPIR")
}

func TestMultiBitMatrixOneMbPIR(t *testing.T) {
	dbLen := oneMB
	blockLen := testBlockLength * field.Bytes
	elemBitSize := 8
	numBlocks := dbLen / (elemBitSize * blockLen)
	nCols := int(math.Sqrt(float64(numBlocks)))
	nRows := nCols

	// functions defined in vpir_test.go
	xofDB := getXof(t, "db key")
	xof := getXof(t, "client key")

	db := database.CreateRandomMultiBitBytes(xofDB, dbLen, nRows, blockLen)

	retrieveBlocksBytes(t, xof, db, numBlocks, "MultiBitMatrixOneMbPIR")
}

func TestDPFMultiBitVectorPIR(t *testing.T) {
	dbLen := oneMB
	blockLen := testBlockLength * field.Bytes
	elemBitSize := 8
	numBlocks := dbLen / (elemBitSize * blockLen)
	nRows := 1

	xofDB := getXof(t, "db key")
	xof := getXof(t, "client key")
	db := database.CreateRandomMultiBitBytes(xofDB, dbLen, nRows, blockLen)

	retrieveBlocksDPFBytes(t, xof, db, numBlocks, "DPFMultiBitVectorPIR")
}

func retrieveBlocksBytes(t *testing.T, rnd io.Reader, db *database.Bytes, numBlocks int, testName string) {
	c := client.NewPIR(rnd, &db.Info)
	s0 := server.NewPIR(db)
	s1 := server.NewPIR(db)

	totalTimer := monitor.NewMonitor()
	for i := 0; i < numBlocks; i++ {
		queries, err := c.QueryBytes(i, 2)
		require.NoError(t, err)

		a0, err := s0.AnswerBytes(queries[0])
		require.NoError(t, err)
		a1, err := s1.AnswerBytes(queries[1])
		require.NoError(t, err)

		answers := [][]byte{a0, a1}

		res, err := c.ReconstructBytes(answers)
		require.NoError(t, err)
		require.Equal(t, db.Entries[i*db.BlockSize:(i+1)*db.BlockSize], res)
	}
	fmt.Printf("TotalCPU time %s: %.2fms\n", testName, totalTimer.Record())
}

func retrieveBlocksDPFBytes(t *testing.T, rnd io.Reader, db *database.Bytes, numBlocks int, testName string) {
	c := client.NewPIRdpf(rnd, &db.Info)
	s0 := server.NewPIRdpf(db)
	s1 := server.NewPIRdpf(db)

	totalTimer := monitor.NewMonitor()
	for i := 0; i < numBlocks; i++ {
		fssKeys, err := c.QueryBytes(i, 2)
		require.NoError(t, err)

		a0, err := s0.AnswerBytes(fssKeys[0])
		require.NoError(t, err)
		a1, err := s1.AnswerBytes(fssKeys[1])
		require.NoError(t, err)

		answers := [][]byte{a0, a1}

		res, err := c.ReconstructBytes(answers)
		require.NoError(t, err)
		require.Equal(t, db.Entries[i*db.BlockSize:(i+1)*db.BlockSize], res)
	}

	fmt.Printf("TotalCPU time %s: %.1fms\n", testName, totalTimer.Record())
}
