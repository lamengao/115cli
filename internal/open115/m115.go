package open115

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
)

type m115Key [16]byte

var (
	m115N, _ = big.NewInt(0).SetString(
		"8686980c0f5a24c4b9d43020cd2c22703ff3f450756529058b1cf88f09b86021"+
			"36477198a6e2683149659bd122c33592fdb5ad47944ad1ea4d36c6b172aad633"+
			"8c3bb6ac6227502d010993ac967d1aef00f0c8e038de2e4d3bc2ec368af2e9f1"+
			"0a6f1eda4f7262f136420c07c331b871bf139f74f3010e3c4fe57df3afb71683", 16)
	m115E, _      = big.NewInt(0).SetString("10001", 16)
	m115KeyLength = m115N.BitLen() / 8

	m115XorKeySeed = []byte{
		0xf0, 0xe5, 0x69, 0xae, 0xbf, 0xdc, 0xbf, 0x8a, 0x1a, 0x45, 0xe8, 0xbe, 0x7d, 0xa6, 0x73, 0xb8,
		0xde, 0x8f, 0xe7, 0xc4, 0x45, 0xda, 0x86, 0xc4, 0x9b, 0x64, 0x8b, 0x14, 0x6a, 0xb4, 0xf1, 0xaa,
		0x38, 0x01, 0x35, 0x9e, 0x26, 0x69, 0x2c, 0x86, 0x00, 0x6b, 0x4f, 0xa5, 0x36, 0x34, 0x62, 0xa6,
		0x2a, 0x96, 0x68, 0x18, 0xf2, 0x4a, 0xfd, 0xbd, 0x6b, 0x97, 0x8f, 0x4d, 0x8f, 0x89, 0x13, 0xb7,
		0x6c, 0x8e, 0x93, 0xed, 0x0e, 0x0d, 0x48, 0x3e, 0xd7, 0x2f, 0x88, 0xd8, 0xfe, 0xfe, 0x7e, 0x86,
		0x50, 0x95, 0x4f, 0xd1, 0xeb, 0x83, 0x26, 0x34, 0xdb, 0x66, 0x7b, 0x9c, 0x7e, 0x9d, 0x7a, 0x81,
		0x32, 0xea, 0xb6, 0x33, 0xde, 0x3a, 0xa9, 0x59, 0x34, 0x66, 0x3b, 0xaa, 0xba, 0x81, 0x60, 0x48,
		0xb9, 0xd5, 0x81, 0x9c, 0xf8, 0x6c, 0x84, 0x77, 0xff, 0x54, 0x78, 0x26, 0x5f, 0xbe, 0xe8, 0x1e,
		0x36, 0x9f, 0x34, 0x80, 0x5c, 0x45, 0x2c, 0x9b, 0x76, 0xd5, 0x1b, 0x8f, 0xcc, 0xc3, 0xb8, 0xf5,
	}
	m115XorClientKey = []byte{0x78, 0x06, 0xad, 0x4c, 0x33, 0x86, 0x5d, 0x18, 0x4c, 0x01, 0x3f, 0x46}
)

func m115GenerateKey() m115Key {
	var key m115Key
	_, _ = io.ReadFull(rand.Reader, key[:])
	return key
}

func m115Encode(input []byte, key m115Key) string {
	buf := make([]byte, 16+len(input))
	copy(buf, key[:])
	copy(buf[16:], input)
	m115XorTransform(buf[16:], m115XorDeriveKey(key[:], 4))
	m115Reverse(buf[16:])
	m115XorTransform(buf[16:], m115XorClientKey)
	return base64.StdEncoding.EncodeToString(m115RSAEncrypt(buf))
}

func m115Decode(input string, key m115Key) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return nil, err
	}
	data = m115RSADecrypt(data)
	if len(data) < 16 {
		return nil, fmt.Errorf("invalid m115 payload")
	}
	output := make([]byte, len(data)-16)
	copy(output, data[16:])
	m115XorTransform(output, m115XorDeriveKey(data[:16], 12))
	m115Reverse(output)
	m115XorTransform(output, m115XorDeriveKey(key[:], 4))
	return output, nil
}

func m115RSAEncrypt(input []byte) []byte {
	buf := &bytes.Buffer{}
	for len(input) > 0 {
		size := m115KeyLength - 11
		if size > len(input) {
			size = len(input)
		}
		m115RSAEncryptSlice(input[:size], buf)
		input = input[size:]
	}
	return buf.Bytes()
}

func m115RSAEncryptSlice(input []byte, w io.Writer) {
	padSize := m115KeyLength - len(input) - 3
	pad := make([]byte, padSize)
	_, _ = rand.Read(pad)
	buf := make([]byte, m115KeyLength)
	buf[0], buf[1] = 0, 2
	for i, b := range pad {
		buf[2+i] = b%0xff + 1
	}
	buf[padSize+2] = 0
	copy(buf[padSize+3:], input)
	ret := big.NewInt(0).Exp(big.NewInt(0).SetBytes(buf), m115E, m115N).Bytes()
	if fill := m115KeyLength - len(ret); fill > 0 {
		_, _ = w.Write(make([]byte, fill))
	}
	_, _ = w.Write(ret)
}

func m115RSADecrypt(input []byte) []byte {
	buf := &bytes.Buffer{}
	for len(input) > 0 {
		size := m115KeyLength
		if size > len(input) {
			size = len(input)
		}
		m115RSADecryptSlice(input[:size], buf)
		input = input[size:]
	}
	return buf.Bytes()
}

func m115RSADecryptSlice(input []byte, w io.Writer) {
	ret := big.NewInt(0).Exp(big.NewInt(0).SetBytes(input), m115E, m115N).Bytes()
	for i, b := range ret {
		if b == 0 && i != 0 {
			_, _ = w.Write(ret[i+1:])
			break
		}
	}
}

func m115XorDeriveKey(seed []byte, size int) []byte {
	key := make([]byte, size)
	for i := range size {
		key[i] = (seed[i] + m115XorKeySeed[size*i]) & 0xff
		key[i] ^= m115XorKeySeed[size*(size-i-1)]
	}
	return key
}

func m115XorTransform(data []byte, key []byte) {
	dataSize, keySize := len(data), len(key)
	mod := dataSize % 4
	for i := 0; i < mod; i++ {
		data[i] ^= key[i%keySize]
	}
	for i := mod; i < dataSize; i++ {
		data[i] ^= key[(i-mod)%keySize]
	}
}

func m115Reverse(data []byte) {
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
}
