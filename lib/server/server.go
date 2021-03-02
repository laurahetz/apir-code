package server

import (
	cst "github.com/si-co/vpir-code/lib/constants"
	"github.com/si-co/vpir-code/lib/database"
	"github.com/si-co/vpir-code/lib/field"
	"math"
	"runtime"
)

// Server is a scheme-agnostic VPIR server interface, implemented by both IT
// and DPF-based schemes
type Server interface {
	AnswerBytes([]byte) ([]byte, error)
	DBInfo() *database.Info
}

// Answer computes the answer for the given query
func answer(q []field.Element, db *database.DB) []field.Element {
	// Doing simplified scheme if block consists of a single bit
	if db.BlockSize == cst.SingleBitBlockLength {
		a := make([]field.Element, db.NumRows)
		for i := 0; i < db.NumRows; i++ {
			for j := 0; j < db.NumColumns; j++ {
				if db.Entries[i][j].Equal(&cst.One) {
					a[i].Add(&a[i], &q[j])
				}
			}
		}
		return a
	}

	// parse the query
	qZeroBase := make([]field.Element, db.NumColumns)
	qOne := make([]field.Element, db.NumColumns*db.BlockSize)
	for j := 0; j < db.NumColumns; j++ {
		qZeroBase[j] = q[j*(db.BlockSize+1)]
		copy(qOne[j*db.BlockSize:(j+1)*db.BlockSize], q[j*(db.BlockSize+1)+1:(j+1)*(db.BlockSize+1)])
	}

	// multithreading
	// channel to pass the chunksChan from the routines back
	numCores := runtime.NumCPU()
	//numCores := 2
	chunksChan := make(chan []field.Element, numCores*(db.BlockSize+1))
	chunkLen := int(math.Ceil(float64(db.NumColumns) / float64(numCores)))
	var numWorkers int
	// compute the matrix-vector inner products
	// addition and multiplication of elements
	// in DB(2^128)^b are executed component-wise
	m := make([]field.Element, db.NumRows*(db.BlockSize+1))
	// we have to traverse column by column
	var begin, end int
	for i := 0; i < db.NumRows; i++ {
		numWorkers = 0
		for j := 0; j < db.NumColumns; j += chunkLen {
			// avoiding overflow when chunkLen does not divide db.Columns evenly
			if j + chunkLen > db.NumColumns {
				chunkLen = db.NumColumns - j
			}
			begin = j*db.BlockSize
			end = (j+chunkLen)*db.BlockSize
			//fmt.Println(begin, end)
			go processChunk(db.Entries[i][begin:end], db.BlockSize, qZeroBase[j:j+chunkLen], qOne[begin:end], chunksChan)
			numWorkers++
		}
		result := combineChunkResults(numWorkers, db.BlockSize+1, chunksChan)
		copy(m[i*(db.BlockSize+1):(i+1)*(db.BlockSize+1)], result)
	}
	close(chunksChan)

	return m
}

// processing a chunk of a database row
func processChunk(dbChunk []field.Element, blockLen int, qZ []field.Element, qO []field.Element, reply chan<- []field.Element) {
	var prodTag, prod field.Element
	sumTag := field.Zero()
	sum := field.ZeroVector(blockLen)
	for j := 0; j < len(dbChunk)/blockLen; j++ {
		for b := 0; b < blockLen; b++ {
			if dbChunk[j*blockLen+b].IsZero() {
				// no need to multiply if the element value is zero
				continue
			}
			// compute message
			prod.Mul(&dbChunk[j*blockLen+b], &qZ[j])
			sum[b].Add(&sum[b], &prod)
			// compute block tag
			prodTag.Mul(&dbChunk[j*blockLen+b], &qO[j*blockLen+b])
			sumTag.Add(&sumTag, &prodTag)
		}
	}
	reply <- append(sum, sumTag)
}

// combine the results of processing a row by different routines
func combineChunkResults(nw int, resLen int, workerReplies <-chan []field.Element) []field.Element {
	product := make([]field.Element, resLen)
	for i := 0; i < nw; i++ {
		reply := <-workerReplies
		for i, elem := range reply {
			product[i].Add(&product[i], &elem)
		}
	}
	return product
}
