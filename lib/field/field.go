package field

// Mostly adapted from:
// https://golang.org/src/crypto/cipher/gcm.go

// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/bits"
	"strconv"

	"golang.org/x/crypto/blake2b"

	our_rand "github.com/si-co/vpir-code/lib/utils"
)

type Element struct {
	value        *gcmFieldElement
	productTable [16]gcmFieldElement
}

func NewElement(in []byte) *Element {
	if len(in) != 16 {
		panic("incorrect length")
	}

	low := binary.BigEndian.Uint64(in[:8])
	high := binary.BigEndian.Uint64(in[8:])

	e := &gcmFieldElement{
		low:  low,
		high: high,
	}

	return &Element{
		value: e,
	}
}

func Zero() *Element {
	v := &gcmFieldElement{low: 0, high: 0}
	return &Element{value: v}
}

func One() *Element {
	one := Zero()
	// the coefficient of x⁰ can be obtained by v.low >> 63.
	one.value.low ^= (1 << 63)
	return one
}

// Generator of the multiplicative group
func Gen() *Element {
	gen := Zero()
	// the coefficient of x^1 can be obtained by v.low >> 62.
	gen.value.low ^= (1 << 62)
	return gen
}

func RandomXOF(xof blake2b.XOF) *Element {
	var bytes [16]byte
	_, err := io.ReadFull(xof, bytes[:])
	if err != nil {
		panic("Should never get here")
	}

	return NewElement(bytes[:])
}

func Random() *Element {
	var bytes [16]byte
	_, err := rand.Read(bytes[:])
	if err != nil {
		panic("Should never get here")
	}

	return NewElement(bytes[:])
}

func RandomVectorXOF(length int, xof blake2b.XOF) []*Element {
	bytesLength := length*16 + 1
	bytes := make([]byte, bytesLength)
	_, err := io.ReadFull(xof, bytes[:])
	if err != nil {
		panic("Should never get here")
	}
	elements := make([]*Element, length)
	for i := 0; i < bytesLength-16; i += 16 {
		elements[i/16] = NewElement(bytes[i : i+16])
	}

	return elements
}

func RandomVectorPRG(length int, prg *our_rand.PRGReader) []*Element {
	bytesLength := length*16 + 1
	bytes := make([]byte, bytesLength)
	_, err := prg.Read(bytes)
	if err != nil {
		panic("Should never get here")
	}
	elements := make([]*Element, length)
	for i := 0; i < bytesLength-16; i += 16 {
		elements[i/16] = NewElement(bytes[i : i+16])
	}

	return elements
}

func Add(x, y *Element) *Element {
	v := gcmAdd(x.value, y.value)
	return &Element{value: &v}
}

func (e *Element) Add(x, y *Element) {
	v := gcmAdd(x.value, y.value)
	*e.value = v
}

// Multiply the two field elements
func Mul(e_in, y_in *Element) *Element {
	e := e_in.value
	y := y_in.value

	productTable := createProductTable(e)
	var z gcmFieldElement

	for i := 0; i < 2; i++ {
		word := y.high
		if i == 1 {
			word = y.low
		}

		// Multiplication works by multiplying z by 16 and adding in
		// one of the precomputed multiples of H.
		for j := 0; j < 64; j += 4 {
			msw := z.high & 0xf
			z.high >>= 4
			z.high |= z.low << 60
			z.low >>= 4
			z.low ^= uint64(gcmReductionTable[msw]) << 48

			// the values in |table| are ordered for
			// little-endian bit positions. See the comment
			// in NewGCMWithNonceSize.
			t := &productTable[word&0xf]

			z.low ^= t.low
			z.high ^= t.high
			word >>= 4
		}
	}

	return &Element{value: &z}
}

func (e *Element) PrecomputeMul() {
	pt := createProductTable(e.value)
	e.productTable = pt
}

// mul e by in and set result in in; e remains unchanged
func (e *Element) MulBy(in *Element) {
	var z gcmFieldElement

	if bits.LeadingZeros64(e.value.low) == 64 &&
		bits.LeadingZeros64(e.value.high) == 64 {

		*in.value = gcmFieldElement{low: 0, high: 0}
		return
	}

	for i := 0; i < 2; i++ {
		word := in.value.high
		if i == 1 {
			word = in.value.low
		}

		// Multiplication works by multiplying z by 16 and adding in
		// one of the precomputed multiples of H.
		for j := 0; j < 64; j += 4 {
			msw := z.high & 0xf
			z.high >>= 4
			z.high |= z.low << 60
			z.low >>= 4
			z.low ^= uint64(gcmReductionTable[msw]) << 48

			// the values in |table| are ordered for
			// little-endian bit positions. See the comment
			// in NewGCMWithNonceSize.
			t := &e.productTable[word&0xf]

			z.low ^= t.low
			z.high ^= t.high
			word >>= 4
		}
	}

	// we need only the value of in, not the product table
	*in.value = z
}

func (e *Element) Mul(x_in, y_in *Element) {
	z := Mul(x_in, y_in)
	e.value = z.value
	e.productTable = z.productTable
}

func (e *Element) Equal(x *Element) bool {
	return e.value.high == x.value.high && e.value.low == x.value.low
}

func (e *Element) String() string {
	return strconv.FormatUint(e.value.low, 16) + strconv.FormatUint(e.value.high, 16)
}

func (e *Element) HexString() string {
	return hex.EncodeToString(e.Bytes())
}

func (e *Element) Bytes() []byte {
	out := make([]byte, 16)
	binary.BigEndian.PutUint64(out[:8], e.value.low)
	binary.BigEndian.PutUint64(out[8:], e.value.high)

	return out
}

func createProductTable(e *gcmFieldElement) [16]gcmFieldElement {
	var productTable [16]gcmFieldElement
	productTable[reverseBits(1)] = *e

	for i := 2; i < 16; i += 2 {
		productTable[reverseBits(i)] = gcmMultiplyByH(&productTable[reverseBits(i/2)])
		productTable[reverseBits(i+1)] = gcmAdd(&productTable[reverseBits(i)], e)
	}

	return productTable
}

// gcmFieldElement represents a value in GF(2¹²⁸).  The bits are stored in big
// endian order. For example:
//   the coefficient of x⁰ can be obtained by v.low >> 63.
//   the coefficient of x⁶³ can be obtained by v.low & 1.
//   the coefficient of x⁶⁴ can be obtained by v.high >> 63.
//   the coefficient of x¹²⁷ can be obtained by v.high & 1.
type gcmFieldElement struct {
	low, high uint64
}

// gcmAdd adds two elements of GF(2¹²⁸) and returns the sum.
func gcmAdd(x, y *gcmFieldElement) gcmFieldElement {
	// Addition in a characteristic 2 field is just XOR.
	return gcmFieldElement{low: x.low ^ y.low, high: x.high ^ y.high}
}

// gcmMultiplyByH returns the result of multiplying an element of GF(2¹²⁸)
// by the element x.
func gcmMultiplyByH(x *gcmFieldElement) (double gcmFieldElement) {
	msbSet := x.high&1 == 1

	// Because of the bit-ordering, doubling is actually a right shift.
	double.high = x.high >> 1
	double.high |= x.low << 63
	double.low = x.low >> 1

	// If the most-significant bit was set before shifting then it,
	// conceptually, becomes a term of x^128. This is greater than the
	// irreducible polynomial so the result has to be reduced. The
	// irreducible polynomial is 1+x+x^2+x^7+x^128. We can subtract that to
	// eliminate the term at x^128 which also means subtracting the other
	// four terms. In characteristic 2 fields, subtraction == addition ==
	// XOR.
	if msbSet {
		double.low ^= 0xe100000000000000
	}

	return
}

var gcmReductionTable = []uint16{
	0x0000, 0x1c20, 0x3840, 0x2460, 0x7080, 0x6ca0, 0x48c0, 0x54e0,
	0xe100, 0xfd20, 0xd940, 0xc560, 0x9180, 0x8da0, 0xa9c0, 0xb5e0,
}

// reverseBits reverses the order of the bits of 4-bit number in i.
func reverseBits(i int) int {
	i = ((i << 2) & 0xc) | ((i >> 2) & 0x3)
	i = ((i << 1) & 0xa) | ((i >> 1) & 0x5)
	return i
}
