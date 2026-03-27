// SPDX-License-Identifier: Apache-2.0

package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"math/big"
)

// ietf1024Prime is the 1024-bit prime for the IETF DH group (RFC 2409 Group 2).
// This is the group used by dh-ietf1024-sha256-aes128-cbc-pkcs7.
var ietf1024Prime, _ = new(big.Int).SetString(
	"FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1"+
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD"+
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245"+
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED"+
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE65381"+
		"FFFFFFFFFFFFFFFF",
	16,
)

// dhGroupSize is the byte length of the IETF 1024-bit DH group prime (128 bytes).
const dhGroupSize = 128

var ietf1024Generator = big.NewInt(2)

// dhGenerateKeyPair generates a private/public key pair for the IETF 1024-bit DH group.
// The private key is a random 256-bit value reduced into [2, p-2].
func dhGenerateKeyPair() (priv, pub *big.Int, err error) {
	privBytes := make([]byte, 32) // 256-bit private exponent
	if _, err = rand.Read(privBytes); err != nil {
		return nil, nil, err
	}
	priv = new(big.Int).SetBytes(privBytes)
	// Reduce into [2, p-2].
	pMinus2 := new(big.Int).Sub(ietf1024Prime, big.NewInt(2))
	priv.Mod(priv, pMinus2)
	priv.Add(priv, big.NewInt(2))

	pub = new(big.Int).Exp(ietf1024Generator, priv, ietf1024Prime)
	return priv, pub, nil
}

// dhDeriveAESKey computes the DH shared secret and derives a 16-byte AES-128 key.
// sharedSecret = peerPubKey^privKey mod p, then aesKey = HKDF-SHA256(sharedSecret)[0:16].
// Uses HKDF (RFC 5869) with an empty salt (32 zero bytes per spec) and empty info,
// matching libsecret's egg_hkdf_perform call in _secret_session_open.
func dhDeriveAESKey(privKey, peerPubKey *big.Int) []byte {
	shared := new(big.Int).Exp(peerPubKey, privKey, ietf1024Prime)

	// Encode the shared secret as a fixed-size big-endian byte array (pad to group size).
	sharedBytes := make([]byte, dhGroupSize)
	b := shared.Bytes()
	copy(sharedBytes[dhGroupSize-len(b):], b)

	// HKDF-Extract: PRK = HMAC-SHA256(salt=zeros[32], IKM=sharedBytes)
	// Per RFC 5869 §2.2: when salt is not provided, use HashLen zero bytes.
	salt := make([]byte, sha256.Size)
	h := hmac.New(sha256.New, salt)
	h.Write(sharedBytes)
	prk := h.Sum(nil)

	// HKDF-Expand: T(1) = HMAC-SHA256(PRK, 0x01)  (info="" and L=16 fit in one round)
	h = hmac.New(sha256.New, prk)
	h.Write([]byte{0x01})
	return h.Sum(nil)[:16]
}

// bigIntToGroupBytes serializes a big.Int to a fixed-size big-endian byte slice padded
// to dhGroupSize (128 bytes), as required for DH public keys on the wire.
func bigIntToGroupBytes(n *big.Int) []byte {
	buf := make([]byte, dhGroupSize)
	b := n.Bytes()
	copy(buf[dhGroupSize-len(b):], b)
	return buf
}

// aesEncrypt encrypts plaintext using AES-128-CBC with PKCS7 padding and a random IV.
// Returns (iv, ciphertext).
func aesEncrypt(key, plaintext []byte) (iv, ciphertext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	iv = make([]byte, aes.BlockSize)
	if _, err = rand.Read(iv); err != nil {
		return nil, nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext = make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	return iv, ciphertext, nil
}

// aesDecrypt decrypts AES-128-CBC ciphertext (PKCS7 padded) using the given key and IV.
func aesDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext length is not a multiple of AES block size")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	return pkcs7Unpad(plaintext)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+padding)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(padding)
	}
	return out
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty padded data")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(data) {
		return nil, errors.New("invalid PKCS7 padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, errors.New("invalid PKCS7 padding byte")
		}
	}
	return data[:len(data)-padding], nil
}
