package server

import (
	"math"
	"runtime"
	"sync"

	"github.com/lukechampine/fastxor"
	cst "github.com/si-co/vpir-code/lib/constants"
	"github.com/si-co/vpir-code/lib/database"
	"github.com/si-co/vpir-code/lib/field"
	"github.com/si-co/vpir-code/lib/utils"
)

// Server is a scheme-agnostic VPIR server interface, implemented by both IT
// and DPF-based schemes
type Server interface {
	AnswerBytes([]byte) ([]byte, error)
	DBInfo() *database.Info
}

/*
%%	VPIR primitives
*/

// Answer computes the VPIR answer for the given query
func answer(q []field.Element, db *database.DB) []field.Element {
	// Doing simplified scheme if block consists of a single bit
	if db.BlockSize == cst.SingleBitBlockLength {
		a := make([]field.Element, db.NumRows)
		for i := 0; i < db.NumRows; i++ {
			for j := 0; j < db.NumColumns; j++ {
				if db.Entries[i*db.NumColumns+j].Equal(&cst.One) {
					a[i].Add(&a[i], &q[j])
				}
			}
		}
		return a
	}

	// %%% Logic %%%
	// compute the matrix-vector inner products,
	// addition and multiplication of elements
	// in DB(2^128)^b are executed component-wise

	// multithreading
	numCores := runtime.NumCPU()
	//numCores := 1
	// If numRows == 1, the db is vector so we split it by giving columns to workers.
	// Otherwise, we split by rows and give a chunk of rows to each worker.
	// The goal is to have a fixed number of workers and start them only once.
	var begin, end int
	if db.NumRows == 1 {
		columnsPerCore := utils.DivideAndRoundUpToMultiple(db.NumColumns, numCores, 1)
		// a channel to pass results from the routines back
		resultsChan := make(chan []field.Element, numCores*(db.BlockSize+1))
		numWorkers := 0
		// we need to traverse column by column
		for j := 0; j < db.NumColumns; j += columnsPerCore {
			columnsPerCore, begin, end = computeChunkIndices(j, columnsPerCore, db.BlockSize, db.NumColumns)
			go processColumns(db.Entries[begin:end], db.BlockSize, q[j*(db.BlockSize+1):(j+columnsPerCore)*(db.BlockSize+1)], resultsChan)
			numWorkers++
		}
		m := combineColumnResults(numWorkers, db.BlockSize+1, resultsChan)
		close(resultsChan)

		return m
	} else {
		m := make([]field.Element, db.NumRows*(db.BlockSize+1))
		var wg sync.WaitGroup
		rowsPerCore := utils.DivideAndRoundUpToMultiple(db.NumRows, numCores, 1)
		for j := 0; j < db.NumRows; j += rowsPerCore {
			rowsPerCore, begin, end = computeChunkIndices(j, rowsPerCore, db.BlockSize, db.NumRows)
			wg.Add(1)
			go processRows(db.Entries[begin*db.NumColumns:end*db.NumColumns], db.BlockSize, db.NumColumns, q, &wg,
				m[j*(db.BlockSize+1):(j+rowsPerCore)*(db.BlockSize+1)])
		}
		wg.Wait()

		return m
	}
}

// processing multiple rows by iterating over them
func processRows(rows []field.Element, blockLen, numColumns int, q []field.Element, wg *sync.WaitGroup, output []field.Element) {
	numElementsInRow := blockLen * numColumns
	for i := 0; i < len(rows)/numElementsInRow; i++ {
		res := computeMessageAndTag(rows[i*numElementsInRow:(i+1)*numElementsInRow], blockLen, q)
		copy(output[i*(blockLen+1):(i+1)*(blockLen+1)], res)
	}
	wg.Done()
}

// processing a chunk of a database row
func processColumns(chunk []field.Element, blockLen int, q []field.Element, reply chan<- []field.Element) {
	reply <- computeMessageAndTag(chunk, blockLen, q)
}

// combine the results of processing a row by different routines
func combineColumnResults(nWrk int, resLen int, workerReplies <-chan []field.Element) []field.Element {
	product := make([]field.Element, resLen)
	for i := 0; i < nWrk; i++ {
		reply := <-workerReplies
		for i, elem := range reply {
			product[i].Add(&product[i], &elem)
		}
	}
	return product
}

// computeMessageAndTag multiplies db entries with the elements
// from the client query and computes a tag over each block
func computeMessageAndTag(elements []field.Element, blockLen int, q []field.Element) []field.Element {
	var prodTag, prod field.Element
	sumTag := field.Zero()
	sum := field.ZeroVector(blockLen)
	for j := 0; j < len(elements)/blockLen; j++ {
		for b := 0; b < blockLen; b++ {
			if elements[j*blockLen+b].IsZero() {
				// no need to multiply if the element value is zero
				continue
			}
			// compute message
			prod.Mul(&elements[j*blockLen+b], &q[j*(blockLen+1)])
			sum[b].Add(&sum[b], &prod)
			// compute block tag
			prodTag.Mul(&elements[j*blockLen+b], &q[j*(blockLen+1)+1+b])
			sumTag.Add(&sumTag, &prodTag)
		}
	}
	return append(sum, sumTag)
}

/*
%%	PIR primitives
*/
func answerPIR(q []byte, db *database.Bytes) []byte {
	m := make([]byte, db.NumRows*db.BlockSize)
	// multithreading
	numCores := runtime.NumCPU()
	var begin, end int
	// Vector db
	if db.NumRows == 1 {
		columnsPerCore := utils.DivideAndRoundUpToMultiple(db.NumColumns, numCores, 8)
		// a channel to pass results from the routines back
		resultsChan := make(chan []byte, numCores*db.BlockSize)
		numWorkers := 0
		for j := 0; j < db.NumColumns; j += columnsPerCore {
			columnsPerCore, begin, end = computeChunkIndices(j, columnsPerCore, db.BlockSize, db.NumColumns)
			// We need /8 because q is packed with 1 bit per block
			go xorColumns(db.Entries[begin:end], db.BlockSize, q[j/8:int(math.Ceil(float64(j+columnsPerCore)/8))], resultsChan)
			numWorkers++
		}
		m = combineColumnXORs(numWorkers, db.BlockSize, resultsChan)
		close(resultsChan)
		return m
	} else {
		//	Matrix db
		var wg sync.WaitGroup
		rowsPerCore := utils.DivideAndRoundUpToMultiple(db.NumRows, numCores, 1)
		for j := 0; j < db.NumRows; j += rowsPerCore {
			rowsPerCore, begin, end = computeChunkIndices(j, rowsPerCore, db.BlockSize, db.NumRows)
			wg.Add(1)
			go xorRows(db.Entries[begin*db.NumColumns:end*db.NumColumns], db.BlockSize, db.NumColumns, q, &wg, m[begin:end])
		}
		wg.Wait()

		return m
	}
}

// XORs entries and q block by block of size bl
func xorValues(entries []byte, bl int, q []byte) []byte {
	sum := make([]byte, bl)
	for j := 0; j < len(entries)/bl; j++ {
		if (q[j/8]>>(j%8))&1 == byte(1) {
			fastxor.Bytes(sum, sum, entries[j*bl:(j+1)*bl])
		}
	}
	return sum
}

// XORs columns in the same row
func xorColumns(columns []byte, blockLen int, q []byte, reply chan<- []byte) {
	reply <- xorValues(columns, blockLen, q)
}

// XORs all the columns in a row, row by row, and writes the result into output
func xorRows(rows []byte, blockLen, numColumns int, q []byte, wg *sync.WaitGroup, output []byte) {
	numElementsInRow := blockLen * numColumns
	for i := 0; i < len(rows)/numElementsInRow; i++ {
		res := xorValues(rows[i*numElementsInRow:(i+1)*numElementsInRow], blockLen, q)
		copy(output[i*blockLen:(i+1)*blockLen], res)
	}
	wg.Done()
}

// Waits for column XORs from individual workers and XORs the results together
func combineColumnXORs(nWrk int, blockLen int, workerReplies <-chan []byte) []byte {
	sum := make([]byte, blockLen)
	for i := 0; i < nWrk; i++ {
		reply := <-workerReplies
		fastxor.Bytes(sum, sum, reply)
	}
	return sum
}

/*
%%	Shared helpers
*/
func computeChunkIndices(ind, step, multiplier, max int) (int, int, int) {
	// avoiding overflow when colPerChunk does not divide db.Columns evenly
	if ind+step > max {
		step = max - ind
	}
	return step, ind * multiplier, (ind + step) * multiplier
}
