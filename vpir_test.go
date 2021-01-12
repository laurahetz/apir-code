package main

import (
	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"log"
	"testing"

	"github.com/si-co/vpir-code/lib/client"
	"github.com/si-co/vpir-code/lib/constants"
	"github.com/si-co/vpir-code/lib/database"
	"github.com/si-co/vpir-code/lib/field"
	"github.com/si-co/vpir-code/lib/monitor"
	"github.com/si-co/vpir-code/lib/server"
	"github.com/si-co/vpir-code/lib/utils"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

func TestRetrieveRandomKeyBlock(t *testing.T) {
	db, blockLength := database.GenerateRandomDB()

	xof, err := blake2b.NewXOF(0, []byte("my key"))
	require.NoError(t, err)
	rebalanced := false

	c := client.NewITMulti(xof, rebalanced)
	s0 := server.NewITMulti(rebalanced, db)
	s1 := server.NewITMulti(rebalanced, db)

	for i := 0; i < 10; i++ {
		queries := c.Query(i, blockLength, 2)

		a0 := s0.Answer(queries[0], blockLength)
		a1 := s1.Answer(queries[1], blockLength)

		answers := [][]field.Element{a0, a1}

		result, err := c.Reconstruct(answers, blockLength)
		require.NoError(t, err)
		fmt.Println(result)
	}
}

func TestRetrieveKey(t *testing.T) {
	db, err := database.FromKeysFile()
	require.NoError(t, err)
	blockLength := 40

	xof, err := blake2b.NewXOF(0, []byte("my key"))
	require.NoError(t, err)
	rebalanced := false

	c := client.NewITMulti(xof, rebalanced)
	s0 := server.NewITMulti(rebalanced, db)
	s1 := server.NewITMulti(rebalanced, db)

	for i := 0; i < 1; i++ {
		queries := c.Query(i, blockLength, 2)

		a0 := s0.Answer(queries[0], blockLength)
		a1 := s1.Answer(queries[1], blockLength)

		answers := [][]field.Element{a0, a1}

		result, err := c.Reconstruct(answers, blockLength)
		require.NoError(t, err)

		// parse result
		// TODO: logic for this should be in lib/gpg
		//lengthBytes := result[0].Bytes()
		//length, _ := binary.Varint(lengthBytes[len(lengthBytes)-1:])

		resultBytes := make([]byte, 0)
		for i := 0; i < len(result); i++ {
			elementBytes := result[i].Bytes()
			//fmt.Println("recon:", elementBytes)
			resultBytes = append(resultBytes, elementBytes[:]...)
		}
		elementsLength, _ := binary.Varint([]byte{resultBytes[0]})
		lastElementLength, _ := binary.Varint([]byte{resultBytes[1]})

		fmt.Println("")
		fmt.Println(elementsLength)
		fmt.Println(lastElementLength)
		fmt.Println(resultBytes[2 : 14+(elementsLength-2)*16+1])

		pub, err := x509.ParsePKIXPublicKey(resultBytes)
		if err != nil {
			log.Printf("failed to parse DER encoded public key: %v", err)
		} else {

			switch pub := pub.(type) {
			case *rsa.PublicKey:
				fmt.Println("pub is of type RSA:", pub)
			case *dsa.PublicKey:
				fmt.Println("pub is of type DSA:", pub)
			case *ecdsa.PublicKey:
				fmt.Println("pub is of type ECDSA:", pub)
			case ed25519.PublicKey:
				fmt.Println("pub is of type Ed25519:", pub)
			default:
				panic("unknown type of public key")
			}
		}
	}
}

func TestMultiBitOneKb(t *testing.T) {
	dbLenMB := 1048576 * 8
	xofDB, err := blake2b.NewXOF(0, []byte("db key"))
	require.NoError(t, err)
	db := database.CreateRandomMultiBitOneMBGF(xofDB, dbLenMB, constants.BlockLength)

	xof, err := blake2b.NewXOF(0, []byte("my key"))
	require.NoError(t, err)
	rebalanced := false

	totalTimer := monitor.NewMonitor()

	c := client.NewITMulti(xof, rebalanced)
	s0 := server.NewITMulti(rebalanced, db)
	s1 := server.NewITMulti(rebalanced, db)

	fieldElements := 128 * 8

	for i := 0; i < fieldElements/16; i++ {
		queries := c.Query(i, constants.BlockLength, 2)

		a0 := s0.Answer(queries[0], constants.BlockLength)
		a1 := s1.Answer(queries[1], constants.BlockLength)

		answers := [][]field.Element{a0, a1}

		res, err := c.Reconstruct(answers, constants.BlockLength)
		require.NoError(t, err)
		require.ElementsMatch(t, db.Entries[i], res)
	}

	fmt.Printf("Total time MultiBitOneKb: %.1fms\n", totalTimer.Record())
}

func TestSingleBitOneKb(t *testing.T) {
	dbLenMB := 1048576 * 8
	blockLen := 1

	xofDB, err := blake2b.NewXOF(0, []byte("db key"))
	require.NoError(t, err)
	db := database.CreateRandomSingleBitDB(xofDB, dbLenMB, blockLen)

	xof, err := blake2b.NewXOF(0, []byte("my key"))
	require.NoError(t, err)

	rebalanced := false

	totalTimer := monitor.NewMonitor()

	c := client.NewITMulti(xof, rebalanced)
	s0 := server.NewITMulti(rebalanced, db)
	s1 := server.NewITMulti(rebalanced, db)

	fieldElements := 128 * 8

	for i := 0; i < fieldElements/16; i++ {
		queries := c.Query(i, blockLen, 2)

		a0 := s0.Answer(queries[0], blockLen)
		a1 := s1.Answer(queries[1], blockLen)

		answers := [][]field.Element{a0, a1}

		_, err := c.Reconstruct(answers, blockLen)
		require.NoError(t, err)
	}

	fmt.Printf("Total time SingleBitOneKb: %.1fms\n", totalTimer.Record())
}

func TestMultiBitVectorGF(t *testing.T) {
	db := database.CreateMultiBitGF()
	xof, err := blake2b.NewXOF(0, []byte("my key"))
	require.NoError(t, err)

	rebalanced := false
	c := client.NewITMulti(xof, rebalanced)
	s0 := server.NewITMulti(rebalanced, db)
	s1 := server.NewITMulti(rebalanced, db)

	i := 0
	queries := c.Query(i, constants.BlockLength, 2)

	a0 := s0.Answer(queries[0], constants.BlockLength)
	a1 := s1.Answer(queries[1], constants.BlockLength)

	answers := [][]field.Element{a0, a1}

	_, err = c.Reconstruct(answers, constants.BlockLength)
	require.NoError(t, err)
}

func TestMatrixOneKbByte(t *testing.T) {
	totalTimer := monitor.NewMonitor()
	db := database.CreateAsciiMatrixOneKbByte()
	xof, err := blake2b.NewXOF(0, []byte("my key"))
	require.NoError(t, err)

	rebalanced := true
	c := client.NewITSingleByte(xof, rebalanced)
	s0 := server.NewITSingleByte(rebalanced, db)
	s1 := server.NewITSingleByte(rebalanced, db)
	s2 := server.NewITSingleByte(rebalanced, db)
	for i := 0; i < 8191; i++ {
		queries := c.Query(i, 3)

		a0 := s0.Answer(queries[0])
		a1 := s1.Answer(queries[1])
		a2 := s2.Answer(queries[2])

		answers := [][]byte{a0, a1, a2}

		_, err := c.Reconstruct(answers)
		require.NoError(t, err)
	}
	fmt.Printf("Total time MatrixOneKbByte: %.1fms\n", totalTimer.Record())
}

func TestMatrixOneKbGF(t *testing.T) {
	totalTimer := monitor.NewMonitor()
	db := database.CreateAsciiMatrixOneKb()
	xof, err := blake2b.NewXOF(0, []byte("my key"))
	if err != nil {
		panic(err)
	}
	rebalanced := true
	c := client.NewITSingleGF(xof, rebalanced)
	s0 := server.NewITSingleGF(rebalanced, db)
	s1 := server.NewITSingleGF(rebalanced, db)
	s2 := server.NewITSingleGF(rebalanced, db)
	for i := 0; i < 8191; i++ {
		queries := c.Query(i, 3)

		a0 := s0.Answer(queries[0])
		a1 := s1.Answer(queries[1])
		a2 := s2.Answer(queries[2])

		answers := [][]field.Element{a0, a1, a2}

		_, err := c.Reconstruct(answers)
		require.NoError(t, err)
	}
	fmt.Printf("Total time MatrixOneKbGF: %.1fms\n", totalTimer.Record())
}

func TestMatrixGF(t *testing.T) {
	totalTimer := monitor.NewMonitor()
	db := database.CreateAsciiMatrixGF()
	result := ""
	xof, err := blake2b.NewXOF(0, []byte("my key"))
	if err != nil {
		panic(err)
	}
	rebalanced := true
	c := client.NewITSingleGF(xof, rebalanced)
	s0 := server.NewITSingleGF(rebalanced, db)
	s1 := server.NewITSingleGF(rebalanced, db)
	s2 := server.NewITSingleGF(rebalanced, db)
	m := monitor.NewMonitor()
	for i := 0; i < 136; i++ {
		m.Reset()
		queries := c.Query(i, 3)
		//fmt.Printf("Query: %.3fms\t", m.RecordAndReset())

		a0 := s0.Answer(queries[0])
		//fmt.Printf("Answer 1: %.3fms\t", m.RecordAndReset())

		a1 := s1.Answer(queries[1])
		//fmt.Printf("Answer 2: %.3fms\t", m.RecordAndReset())

		a2 := s2.Answer(queries[2])
		//fmt.Printf("Answer 3: %.3fms\t", m.RecordAndReset())

		answers := [][]field.Element{a0, a1, a2}

		m.Reset()
		x, err := c.Reconstruct(answers)
		require.NoError(t, err)
		//fmt.Printf("Reconstruct: %.3fms\n", m.RecordAndReset())
		if x.String() == "0" {
			result += "0"
		} else {
			result += "1"
		}
	}
	fmt.Printf("\n\n")

	b, err := utils.BitStringToBytes(result)
	if err != nil {
		t.Error(err)
		panic(err)
	}

	output := string(b)
	fmt.Println(output)

	const expected = "Playing with VPIR"
	if expected != output {
		t.Errorf("Expected '%v' but got '%v'", expected, output)
	}

	fmt.Printf("Total time: %.1fms\n", totalTimer.Record())
}

func TestVectorGF(t *testing.T) {
	totalTimer := monitor.NewMonitor()
	db := database.CreateAsciiVectorGF()
	result := ""
	xof, err := blake2b.NewXOF(0, []byte("my key"))
	if err != nil {
		panic(err)
	}
	rebalanced := false
	c := client.NewITSingleGF(xof, rebalanced)
	s0 := server.NewITSingleGF(rebalanced, db)
	s1 := server.NewITSingleGF(rebalanced, db)
	s2 := server.NewITSingleGF(rebalanced, db)
	m := monitor.NewMonitor()
	for i := 0; i < 136; i++ {
		m.Reset()
		queries := c.Query(i, 3)
		//fmt.Printf("Query: %.3fms\t", m.RecordAndReset())

		a0 := s0.Answer(queries[0])
		//fmt.Printf("Answer 1: %.3fms\t", m.RecordAndReset())

		a1 := s1.Answer(queries[1])
		//fmt.Printf("Answer 2: %.3fms\t", m.RecordAndReset())

		a2 := s2.Answer(queries[2])
		//fmt.Printf("Answer 3: %.3fms\t", m.RecordAndReset())

		answers := [][]field.Element{a0, a1, a2}

		m.Reset()
		x, err := c.Reconstruct(answers)
		require.NoError(t, err)
		//fmt.Printf("Reconstruct: %.3fms\n", m.RecordAndReset())
		if x.String() == "0" {
			result += "0"
		} else {
			result += "1"
		}
	}
	b, err := utils.BitStringToBytes(result)
	if err != nil {
		t.Error(err)
		panic(err)
	}

	output := string(b)
	fmt.Println(output)

	const expected = "Playing with VPIR"
	if expected != output {
		t.Errorf("Expected '%v' but got '%v'", expected, output)
	}

	fmt.Printf("Total time VectorGF: %.1fms\n", totalTimer.Record())
}

func TestVectorByte(t *testing.T) {
	totalTimer := monitor.NewMonitor()
	db := database.CreateAsciiVectorByte()
	result := ""
	xof, err := blake2b.NewXOF(0, []byte("my key"))
	if err != nil {
		panic(err)
	}
	rebalanced := false
	c := client.NewITSingleByte(xof, rebalanced)
	s0 := server.NewITSingleByte(rebalanced, db)
	s1 := server.NewITSingleByte(rebalanced, db)
	s2 := server.NewITSingleByte(rebalanced, db)
	m := monitor.NewMonitor()
	for i := 0; i < 136; i++ {
		m.Reset()
		queries := c.Query(i, 3)
		fmt.Printf("Query: %.3fms\t", m.RecordAndReset())

		a0 := s0.Answer(queries[0])
		fmt.Printf("Answer 1: %.3fms\t", m.RecordAndReset())

		a1 := s1.Answer(queries[1])
		fmt.Printf("Answer 2: %.3fms\t", m.RecordAndReset())

		a2 := s2.Answer(queries[2])
		fmt.Printf("Answer 3: %.3fms\t", m.RecordAndReset())

		answers := [][]byte{a0, a1, a2}

		m.Reset()
		x, err := c.Reconstruct(answers)
		fmt.Println(x)
		require.NoError(t, err)
		fmt.Printf("Reconstruct: %.3fms\n", m.RecordAndReset())
		if x == byte(0) {
			result += "0"
		} else {
			result += "1"
		}
	}
	b, err := utils.BitStringToBytes(result)
	if err != nil {
		t.Error(err)
		panic(err)
	}

	output := string(b)
	fmt.Println(output)

	const expected = "Playing with VPIR"
	if expected != output {
		t.Errorf("Expected '%v' but got '%v'", expected, output)
	}

	fmt.Printf("Total time VectorByte: %.1fms\n", totalTimer.Record())
}

func TestDPF(t *testing.T) {
	totalTimer := monitor.NewMonitor()
	db := database.CreateAsciiVectorGF()
	result := ""
	xof, err := blake2b.NewXOF(0, []byte("my key"))
	if err != nil {
		panic(err)
	}
	c := client.NewDPF(xof)
	s0 := server.NewDPFServer(db)
	s1 := server.NewDPFServer(db)
	m := monitor.NewMonitor()

	for i := 0; i < 136; i++ {
		m.Reset()
		prfKeys, fssKeys := c.Query(i, 2)
		fmt.Printf("Query: %.3fms\t", m.RecordAndReset())

		a0 := s0.Answer(fssKeys[0], prfKeys, 0)
		fmt.Printf("Answer 1: %.3fms\t", m.RecordAndReset())

		a1 := s1.Answer(fssKeys[1], prfKeys, 1)
		fmt.Printf("Answer 2: %.3fms\t", m.RecordAndReset())

		answers := [][]field.Element{a0, a1}

		m.Reset()
		x, err := c.Reconstruct(answers)
		fmt.Printf("Reconstruct: %.3fms\n", m.RecordAndReset())
		if err != nil {
			panic(err)
		}
		if x[0].String() == "0" {
			result += "0"
		} else {
			result += "1"
		}

	}
	b, err := utils.BitStringToBytes(result)
	if err != nil {
		t.Error(err)
		panic(err)
	}

	output := string(b)
	fmt.Println(output)

	const expected = "Playing with VPIR"
	if expected != output {
		t.Errorf("Expected '%v' but got '%v'", expected, output)
	}

	fmt.Printf("Total time: %.1fms\n", totalTimer.Record())
}
