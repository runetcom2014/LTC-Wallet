package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/ripemd160"

	bip39 "github.com/tyler-smith/go-bip39"
)

// LTC BIP44 путь: m/44'/2'/0'/0/0
const (
	ltcCoinType  = 2
	bip44Purpose = 44
)

// LTC mainnet версии для адресов и extended keys
var (
	ltcP2PKHVersion = []byte{0x30}        // адреса начинаются с 'L'
	ltcXPRVVersion  = []byte{0x01, 0x9D, 0xA4, 0x62} // zprv для LTC
)

// WalletKeys — все ключи кошелька
type WalletKeys struct {
	Mnemonic   string
	Address    string
	PublicKey  []byte
	PrivateKey []byte
}

// DeriveWallet деривирует LTC кошелёк из мастер ключа ритуала
func DeriveWallet(master [32]byte) (WalletKeys, error) {
	// Шаг 1: HKDF → энтропия для BIP39
	h := hkdf.New(sha256.New, master[:], nil, []byte("87d32c69ac183b7832e01cf5:RITUAL-V1:bip39"))
	entropy := make([]byte, 32)
	if _, err := io.ReadFull(h, entropy); err != nil {
		return WalletKeys{}, fmt.Errorf("hkdf: %w", err)
	}

	// Шаг 2: BIP39 мнемоника из энтропии
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return WalletKeys{}, fmt.Errorf("bip39: %w", err)
	}

	// Шаг 3: BIP39 seed из мнемоники (без passphrase)
	seed := pbkdf2.Key([]byte(mnemonic), []byte("mnemonic"), 2048, 64, sha512.New)

	// Шаг 4: BIP32 мастер ключ из seed
	masterKey, masterChain, err := newMasterKey(seed)
	if err != nil {
		return WalletKeys{}, fmt.Errorf("master key: %w", err)
	}

	// Шаг 5: деривация по пути m/44'/2'/0'/0/0
	path := []uint32{
		0x80000000 + bip44Purpose, // 44' hardened
		0x80000000 + ltcCoinType,  // 2'  hardened
		0x80000000 + 0,            // 0'  hardened (account)
		0,                         // 0   external chain
		0,                         // 0   первый адрес
	}

	privKey, chainCode := masterKey, masterChain
	for _, index := range path {
		privKey, chainCode, err = deriveChild(privKey, chainCode, index)
		if err != nil {
			return WalletKeys{}, fmt.Errorf("derive child: %w", err)
		}
	}

	// Шаг 6: публичный ключ из приватного
	pubKey, err := privKeyToPubKey(privKey)
	if err != nil {
		return WalletKeys{}, fmt.Errorf("pubkey: %w", err)
	}

	// Шаг 7: LTC P2PKH адрес
	address, err := pubKeyToLTCAddress(pubKey)
	if err != nil {
		return WalletKeys{}, fmt.Errorf("address: %w", err)
	}

	return WalletKeys{
		Mnemonic:   mnemonic,
		Address:    address,
		PublicKey:  pubKey,
		PrivateKey: privKey,
	}, nil
}

// newMasterKey создаёт BIP32 мастер ключ из seed
func newMasterKey(seed []byte) ([]byte, []byte, error) {
	mac := hmac.New(sha512.New, []byte("Bitcoin seed"))
	mac.Write(seed)
	I := mac.Sum(nil)
	IL, IR := I[:32], I[32:]
	if !isValidPrivKey(IL) {
		return nil, nil, errors.New("invalid master key")
	}
	return IL, IR, nil
}

// deriveChild деривирует дочерний ключ (hardened если index >= 0x80000000)
func deriveChild(privKey, chainCode []byte, index uint32) ([]byte, []byte, error) {
	var data []byte
	if index >= 0x80000000 {
		// hardened: 0x00 || privKey || index
		data = append([]byte{0x00}, privKey...)
	} else {
		// normal: pubKey || index
		pubKey, err := privKeyToPubKey(privKey)
		if err != nil {
			return nil, nil, err
		}
		data = pubKey
	}
	indexBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(indexBytes, index)
	data = append(data, indexBytes...)

	mac := hmac.New(sha512.New, chainCode)
	mac.Write(data)
	I := mac.Sum(nil)
	IL, IR := I[:32], I[32:]

	// childKey = (IL + parentKey) mod n
	curve := secp256k1()
	n := curve.N
	childKeyInt := new(big.Int).SetBytes(IL)
	parentKeyInt := new(big.Int).SetBytes(privKey)
	childKeyInt.Add(childKeyInt, parentKeyInt)
	childKeyInt.Mod(childKeyInt, n)

	if childKeyInt.Sign() == 0 || !isValidPrivKey(IL) {
		return nil, nil, errors.New("invalid child key")
	}

	childKey := make([]byte, 32)
	childKeyInt.FillBytes(childKey)
	return childKey, IR, nil
}

// privKeyToPubKey возвращает сжатый публичный ключ (33 байта)
func privKeyToPubKey(privKey []byte) ([]byte, error) {
	curve := secp256k1()
	privInt := new(big.Int).SetBytes(privKey)
	x, y := curve.ScalarBaseMult(privInt.Bytes())
	// сжатый формат: 02 или 03 + x
	prefix := byte(0x02)
	if y.Bit(0) == 1 {
		prefix = 0x03
	}
	pubKey := make([]byte, 33)
	pubKey[0] = prefix
	x.FillBytes(pubKey[1:])
	return pubKey, nil
}

// pubKeyToLTCAddress конвертирует публичный ключ в LTC P2PKH адрес
func pubKeyToLTCAddress(pubKey []byte) (string, error) {
	// SHA256 → RIPEMD160 (hash160)
	sha := sha256.Sum256(pubKey)
	rmd := ripemd160.New()
	rmd.Write(sha[:])
	hash160 := rmd.Sum(nil)

	// version byte + hash160
	payload := append(ltcP2PKHVersion, hash160...)

	// double SHA256 checksum
	cs1 := sha256.Sum256(payload)
	cs2 := sha256.Sum256(cs1[:])
	checksum := cs2[:4]

	// Base58Check
	full := append(payload, checksum...)
	return base58Encode(full), nil
}

// isValidPrivKey проверяет что ключ в диапазоне [1, n-1]
func isValidPrivKey(key []byte) bool {
	curve := secp256k1()
	k := new(big.Int).SetBytes(key)
	return k.Sign() > 0 && k.Cmp(curve.N) < 0
}

// secp256k1Curve — параметры кривой secp256k1
type secp256k1Curve struct {
	P, N, B *big.Int
	Gx, Gy  *big.Int
	BitSize int
}

func (c *secp256k1Curve) Params() *secp256k1Curve { return c }

func (c *secp256k1Curve) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	return scalarMult(c.Gx, c.Gy, k, c)
}

func secp256k1() *secp256k1Curve {
	p, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEFFFFFC2F", 16)
	n, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	b, _ := new(big.Int).SetString("0000000000000000000000000000000000000000000000000000000000000007", 16)
	gx, _ := new(big.Int).SetString("79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798", 16)
	gy, _ := new(big.Int).SetString("483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8", 16)
	return &secp256k1Curve{P: p, N: n, B: b, Gx: gx, Gy: gy, BitSize: 256}
}

// scalarMult — умножение точки на скаляр (double-and-add)
func scalarMult(Bx, By *big.Int, k []byte, curve *secp256k1Curve) (*big.Int, *big.Int) {
	Bx = new(big.Int).Set(Bx)
	By = new(big.Int).Set(By)
	scalar := new(big.Int).SetBytes(k)
	rx, ry := new(big.Int), new(big.Int)
	px, py := new(big.Int).Set(Bx), new(big.Int).Set(By)
	for i := 0; i < scalar.BitLen(); i++ {
		if scalar.Bit(i) == 1 {
			rx, ry = pointAdd(rx, ry, px, py, curve.P)
		}
		px, py = pointDouble(px, py, curve.P)
	}
	return rx, ry
}

func pointAdd(x1, y1, x2, y2, p *big.Int) (*big.Int, *big.Int) {
	if x1.Sign() == 0 && y1.Sign() == 0 {
		return new(big.Int).Set(x2), new(big.Int).Set(y2)
	}
	if x2.Sign() == 0 && y2.Sign() == 0 {
		return new(big.Int).Set(x1), new(big.Int).Set(y1)
	}
	dx := new(big.Int).Sub(x2, x1)
	dy := new(big.Int).Sub(y2, y1)
	dx.Mod(dx, p)
	if dx.Sign() < 0 {
		dx.Add(dx, p)
	}
	inv := new(big.Int).ModInverse(dx, p)
	lam := new(big.Int).Mul(dy, inv)
	lam.Mod(lam, p)
	x3 := new(big.Int).Mul(lam, lam)
	x3.Sub(x3, x1)
	x3.Sub(x3, x2)
	x3.Mod(x3, p)
	y3 := new(big.Int).Sub(x1, x3)
	y3.Mul(lam, y3)
	y3.Sub(y3, y1)
	y3.Mod(y3, p)
	if x3.Sign() < 0 {
		x3.Add(x3, p)
	}
	if y3.Sign() < 0 {
		y3.Add(y3, p)
	}
	return x3, y3
}

func pointDouble(x, y, p *big.Int) (*big.Int, *big.Int) {
	if x.Sign() == 0 && y.Sign() == 0 {
		return new(big.Int), new(big.Int)
	}
	lam := new(big.Int).Mul(x, x)
	lam.Mul(lam, big.NewInt(3))
	inv := new(big.Int).Mul(y, big.NewInt(2))
	inv.Mod(inv, p)
	inv.ModInverse(inv, p)
	lam.Mul(lam, inv)
	lam.Mod(lam, p)
	x3 := new(big.Int).Mul(lam, lam)
	x3.Sub(x3, new(big.Int).Mul(big.NewInt(2), x))
	x3.Mod(x3, p)
	y3 := new(big.Int).Sub(x, x3)
	y3.Mul(lam, y3)
	y3.Sub(y3, y)
	y3.Mod(y3, p)
	if x3.Sign() < 0 {
		x3.Add(x3, p)
	}
	if y3.Sign() < 0 {
		y3.Add(y3, p)
	}
	return x3, y3
}

// base58Encode кодирует байты в Base58
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Encode(input []byte) string {
	n := new(big.Int).SetBytes(input)
	zero := big.NewInt(0)
	mod := new(big.Int)
	var result []byte
	for n.Cmp(zero) > 0 {
		n.DivMod(n, big.NewInt(58), mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}
	// ведущие нули
	for _, b := range input {
		if b != 0 {
			break
		}
		result = append(result, base58Alphabet[0])
	}
	// разворачиваем
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}
