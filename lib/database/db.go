package database

import (
	"bytes"
	"crypto"
	"encoding/binary"
	"encoding/gob"
	"io"
	"log"
	"math"

	"github.com/cloudflare/circl/group"
	"github.com/ldsec/lattigo/v2/bfv"
	"github.com/si-co/vpir-code/lib/field"
	"github.com/si-co/vpir-code/lib/utils"
	"go.etcd.io/bbolt"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/xerrors"
)

var DefaultChunkSize = 1e7

const infoDbKey = "info"

func NewDB(info Info) (*DB, error) {
	n := info.BlockSize * info.NumColumns * info.NumRows
	if info.BlockSize == 0 {
		n = info.NumColumns * info.NumRows
	}

	return &DB{
		Info:     info,
		inMemory: make([]field.Element, n),
	}, nil
}

type DB struct {
	Info
	inMemory []field.Element
}

func (d *DB) SetEntry(i int, el field.Element) {
	d.inMemory[i] = el
}

type saveInfo struct {
	Info Info
	// the list of chunks, with start/end indexes for each chunk
	Chunks [][2]int
}

func (d *DB) SaveDB(path string, bucket string) error {
	chunkSize := DefaultChunkSize

	db, err := bbolt.Open(path, 0666, nil)
	if err != nil {
		return xerrors.Errorf("failed to open db: %v", err)
	}

	defer db.Close()

	err = db.Update(func(t *bbolt.Tx) error {
		_, err := t.CreateBucket([]byte(bucket))
		if err != nil {
			return xerrors.Errorf("failed to create bucket: %v", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("failed to create bucket: %v", err)
	}

	saveInfo := saveInfo{
		Info:   d.Info,
		Chunks: make([][2]int, 0),
	}

	n := d.Info.BlockSize * d.Info.NumColumns * d.Info.NumRows

	err = db.Update(func(t *bbolt.Tx) error {
		for i := 0; i < n; i += int(chunkSize) {
			key := make([]byte, 8)
			binary.LittleEndian.PutUint64(key, uint64(i))

			var chunk []field.Element
			if i+int(chunkSize) >= n {
				chunk = d.inMemory[i:]
				log.Println("saving last chunk")
			} else {
				chunk = d.inMemory[i : i+int(chunkSize)]
			}

			buf := new(bytes.Buffer)
			enc := gob.NewEncoder(buf)

			err := enc.Encode(chunk)
			if err != nil {
				return xerrors.Errorf("failed to encode chunk: %v", err)
			}

			log.Println("saving chunk", i, i+len(chunk))
			saveInfo.Chunks = append(saveInfo.Chunks, [2]int{i, i + len(chunk)})

			err = t.Bucket([]byte(bucket)).Put(key, buf.Bytes())
			if err != nil {
				return xerrors.Errorf("failed to put chunk: %v", err)
			}

		}

		buf := new(bytes.Buffer)
		enc := gob.NewEncoder(buf)

		err := enc.Encode(&saveInfo)
		if err != nil {
			return xerrors.Errorf("failed to encode info: %v", err)
		}

		err = t.Bucket([]byte(bucket)).Put([]byte(infoDbKey), buf.Bytes())
		if err != nil {
			return xerrors.Errorf("failed to put info: %v", err)
		}

		return nil
	})

	if err != nil {
		return xerrors.Errorf("failed to save chunks: %v", err)
	}

	return nil
}

func LoadDB(path, bucket string) (*DB, error) {
	db, err := bbolt.Open(path, 0666, nil)
	if err != nil {
		return nil, xerrors.Errorf("failed to open db: %v", err)
	}

	defer db.Close()

	var elements []field.Element
	var info Info

	err = db.View(func(t *bbolt.Tx) error {

		res := t.Bucket([]byte(bucket)).Get([]byte(infoDbKey))
		buf := bytes.NewBuffer(res)
		dec := gob.NewDecoder(buf)

		saveInfo := saveInfo{}

		err := dec.Decode(&saveInfo)
		if err != nil {
			return xerrors.Errorf("failed to decode info: %v", err)
		}

		info = saveInfo.Info
		n := info.BlockSize * info.NumColumns * info.NumRows

		elements = make([]field.Element, n)

		for _, i := range saveInfo.Chunks {
			start, end := i[0], i[1]

			chunk := make([]field.Element, end-start)

			key := make([]byte, 8)
			binary.LittleEndian.PutUint64(key, uint64(start))

			res := t.Bucket([]byte(bucket)).Get(key)
			buf := bytes.NewBuffer(res)

			dec := gob.NewDecoder(buf)
			err = dec.Decode(&chunk)
			if err != nil {
				return xerrors.Errorf("failed to decode chunk: %v", err)
			}

			log.Println("loading", start, start+len(chunk))
			copy(elements[start:start+len(chunk)], chunk)
		}

		return nil
	})

	if err != nil {
		return nil, xerrors.Errorf("failed to read db: %v", err)
	}

	result := DB{
		inMemory: elements,
		Info:     info,
	}

	return &result, nil
}

func (d *DB) GetEntry(i int) field.Element {
	return d.inMemory[i]
}

func (d *DB) Range(begin, end int) []field.Element {
	return d.inMemory[begin:end]
}

type Info struct {
	NumRows    int
	NumColumns int
	BlockSize  int

	// PIR type: classical, merkle, signature
	PIRType string

	*Auth
	*Merkle
	*DataEmbedding

	//Lattice parameters for the single-server data retrieval
	LatParams *bfv.Parameters
}

// Authentication information for the single-server setting
type Auth struct {
	// The global digest that is a hash of all the row digests. Public.
	Digest []byte
	// ECC group and hash algorithm used for digest computation and PIR itself
	Group group.Group
	Hash  crypto.Hash
	// Due to lack of the size functions in the lib API, we store it in the db info
	ElementSize int
	ScalarSize  int
}

// Data embedding info
type DataEmbedding struct {
	IDLength  int
	KeyLength int
}

// The info needed for the Merkle-tree based approach
type Merkle struct {
	Root     []byte
	ProofLen int
}

func CreateZeroMultiBitDB(numRows, numColumns, blockSize int) (*DB, error) {
	info := Info{NumColumns: numColumns,
		NumRows:   numRows,
		BlockSize: blockSize,
	}

	db, err := NewDB(info)
	if err != nil {
		return nil, xerrors.Errorf("failed to create db: %v", err)
	}

	n := numRows * numColumns * blockSize
	for i := 0; i < n; i++ {
		db.SetEntry(i, field.Zero())
	}

	return db, nil
}

func CreateRandomMultiBitDB(rnd io.Reader, dbLen, numRows, blockLen int) (*DB, error) {
	numColumns := dbLen / (8 * field.Bytes * numRows * blockLen)
	// handle very small db
	if numColumns == 0 {
		numColumns = 1
	}

	info := Info{
		NumColumns: numColumns,
		NumRows:    numRows,
		BlockSize:  blockLen,
	}

	n := numRows * numColumns * blockLen

	bytesLength := n*field.Bytes + 1
	bytes := make([]byte, bytesLength)

	// not sure that getting all random bytes at once is a good idea
	_, err := io.ReadFull(rnd, bytes[:])
	if err != nil {
		return nil, xerrors.Errorf("failed to read random bytes: %v", err)
	}

	db, err := NewDB(info)
	if err != nil {
		return nil, xerrors.Errorf("failed to create db: %v", err)
	}

	for i := 0; i < n; i++ {
		var buf [16]byte
		copy(buf[:], bytes[i*field.Bytes:(1+i)*field.Bytes])
		element := &field.Element{}
		element.SetFixedLengthBytes(buf)

		db.SetEntry(i, *element)
	}

	return db, nil
}

func CreateRandomSingleBitDB(rnd io.Reader, dbLen, numRows int) (*DB, error) {
	numColumns := dbLen / numRows

	// by convention a block size of 0 indicates the single-bit scheme
	info := Info{NumColumns: numColumns, NumRows: numRows, BlockSize: 0}

	db, err := NewDB(info)
	if err != nil {
		return nil, xerrors.Errorf("failed to create db: %v", err)
	}

	for i := 0; i < dbLen; i++ {
		element := field.Element{}
		element.SetRandom(rnd)

		tmpb := element.Bytes()[len(element.Bytes())-1]
		if tmpb>>7 == 1 {
			element.SetOne()
		} else {
			element.SetZero()
		}

		db.SetEntry(i, element)
	}

	return db, nil
}

// HashToIndex hashes the given id to an index for a database of the given
// length
func HashToIndex(id string, length int) int {
	hash := blake2b.Sum256([]byte(id))
	return int(binary.BigEndian.Uint64(hash[:]) % uint64(length))
}

func CalculateNumRowsAndColumns(numBlocks int, matrix bool) (numRows, numColumns int) {
	utils.IncreaseToNextSquare(&numBlocks)
	if matrix {
		numColumns = int(math.Sqrt(float64(numBlocks)))
		numRows = numColumns
	} else {
		numColumns = numBlocks
		numRows = 1
	}
	return
}
