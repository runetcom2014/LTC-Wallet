package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"golang.org/x/crypto/ripemd160"
)

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// TxOutput — выход транзакции
type TxOutput struct {
	Address string
	Value   int64 // satoshi
}

// BuildAndSign собирает и подписывает P2PKH транзакцию
// Возвращает hex транзакции готовой к broadcast
func BuildAndSign(utxos []UTXO, outputs []TxOutput, privKey []byte, pubKey []byte) (string, error) {
	if len(utxos) == 0 {
		return "", errors.New("нет UTXOs")
	}
	if len(outputs) == 0 {
		return "", errors.New("нет получателей")
	}

	// --- сборка транзакции для подписания ---
	// подписываем каждый input отдельно (SIGHASH_ALL)
	signedInputs := make([][]byte, len(utxos))

	for i := range utxos {
		// строим preimage для подписания i-го input
		preimage, err := buildPreimage(utxos, outputs, i, pubKey)
		if err != nil {
			return "", fmt.Errorf("preimage[%d]: %w", i, err)
		}

		// double SHA256
		h1 := sha256.Sum256(preimage)
		h2 := sha256.Sum256(h1[:])

		// ECDSA подпись
		sig, err := ecdsaSign(h2[:], privKey)
		if err != nil {
			return "", fmt.Errorf("sign[%d]: %w", i, err)
		}
		// добавляем SIGHASH_ALL
		sig = append(sig, 0x01)

		// scriptSig: <sig> <pubKey>
		scriptSig := buildScriptSig(sig, pubKey)
		signedInputs[i] = scriptSig
	}

	// --- финальная транзакция ---
	tx := serializeTx(utxos, outputs, signedInputs)
	return hex.EncodeToString(tx), nil
}

// buildPreimage строит данные для подписания input[sigIdx] (BIP143 не нужен для P2PKH legacy)
func buildPreimage(utxos []UTXO, outputs []TxOutput, sigIdx int, pubKey []byte) ([]byte, error) {
	var buf []byte

	// version (4 bytes LE)
	buf = appendUint32LE(buf, 1)

	// inputs
	buf = appendVarInt(buf, uint64(len(utxos)))
	for i := range utxos {
		// txid (reversed)
		txid, err := hex.DecodeString(utxos[i].TxHash)
		if err != nil {
			return nil, fmt.Errorf("txid decode: %w", err)
		}
		reverseBytes(txid)
		buf = append(buf, txid...)

		// vout (4 bytes LE)
		buf = appendUint32LE(buf, uint32(utxos[i].TxPos))

		if i == sigIdx {
			// подписываемый input: ставим P2PKH scriptPubKey
			script := p2pkhScript(pubKey)
			buf = appendVarInt(buf, uint64(len(script)))
			buf = append(buf, script...)
		} else {
			// остальные inputs: пустой script
			buf = append(buf, 0x00)
		}

		// sequence (4 bytes LE)
		buf = appendUint32LE(buf, 0xffffffff)
	}

	// outputs
	buf = appendVarInt(buf, uint64(len(outputs)))
	for _, out := range outputs {
		// value (8 bytes LE)
		buf = appendUint64LE(buf, uint64(out.Value))

		// scriptPubKey для адреса получателя
		script, err := addressToScript(out.Address)
		if err != nil {
			return nil, fmt.Errorf("output script: %w", err)
		}
		buf = appendVarInt(buf, uint64(len(script)))
		buf = append(buf, script...)
	}

	// locktime (4 bytes LE)
	buf = appendUint32LE(buf, 0)

	// SIGHASH_ALL (4 bytes LE)
	buf = appendUint32LE(buf, 1)

	return buf, nil
}

// serializeTx собирает финальную подписанную транзакцию
func serializeTx(utxos []UTXO, outputs []TxOutput, signedInputs [][]byte) []byte {
	var buf []byte

	buf = appendUint32LE(buf, 1) // version

	// inputs
	buf = appendVarInt(buf, uint64(len(utxos)))
	for i := range utxos {
		txid, _ := hex.DecodeString(utxos[i].TxHash)
		reverseBytes(txid)
		buf = append(buf, txid...)
		buf = appendUint32LE(buf, uint32(utxos[i].TxPos))
		scriptSig := signedInputs[i]
		buf = appendVarInt(buf, uint64(len(scriptSig)))
		buf = append(buf, scriptSig...)
		buf = appendUint32LE(buf, 0xffffffff)
	}

	// outputs
	buf = appendVarInt(buf, uint64(len(outputs)))
	for _, out := range outputs {
		buf = appendUint64LE(buf, uint64(out.Value))
		script, _ := addressToScript(out.Address)
		buf = appendVarInt(buf, uint64(len(script)))
		buf = append(buf, script...)
	}

	buf = appendUint32LE(buf, 0) // locktime
	return buf
}

// p2pkhScript строит P2PKH scriptPubKey из публичного ключа
func p2pkhScript(pubKey []byte) []byte {
	h160 := hash160(pubKey)
	script := make([]byte, 25)
	script[0] = 0x76 // OP_DUP
	script[1] = 0xa9 // OP_HASH160
	script[2] = 0x14 // push 20 bytes
	copy(script[3:23], h160)
	script[23] = 0x88 // OP_EQUALVERIFY
	script[24] = 0xac // OP_CHECKSIG
	return script
}

// addressToScript конвертирует адрес в scriptPubKey
// поддерживает P2PKH (L-адреса, версия 0x30) и P2SH (M-адреса, версия 0x32)
func addressToScript(address string) ([]byte, error) {
	decoded, err := base58Decode(address)
	if err != nil {
		return nil, err
	}
	if len(decoded) < 21 {
		return nil, errors.New("invalid address")
	}
	version := decoded[0]
	hash160 := decoded[1:21]

	switch version {
	case 0x30: // P2PKH — адреса начинаются с 'L'
		script := make([]byte, 25)
		script[0] = 0x76 // OP_DUP
		script[1] = 0xa9 // OP_HASH160
		script[2] = 0x14 // push 20 bytes
		copy(script[3:23], hash160)
		script[23] = 0x88 // OP_EQUALVERIFY
		script[24] = 0xac // OP_CHECKSIG
		return script, nil
	case 0x32: // P2SH — адреса начинаются с 'M'
		script := make([]byte, 23)
		script[0] = 0xa9 // OP_HASH160
		script[1] = 0x14 // push 20 bytes
		copy(script[2:22], hash160)
		script[22] = 0x87 // OP_EQUAL
		return script, nil
	default:
		return nil, fmt.Errorf("unsupported address version: 0x%02x", version)
	}
}

// buildScriptSig строит scriptSig: <sig_len> <sig> <pubkey_len> <pubkey>
func buildScriptSig(sig, pubKey []byte) []byte {
	buf := make([]byte, 0, 1+len(sig)+1+len(pubKey))
	buf = append(buf, byte(len(sig)))
	buf = append(buf, sig...)
	buf = append(buf, byte(len(pubKey)))
	buf = append(buf, pubKey...)
	return buf
}

// ecdsaSign подписывает хеш приватным ключом (secp256k1 ECDSA, DER формат)
func ecdsaSign(hash, privKey []byte) ([]byte, error) {
	curve := secp256k1()
	n := curve.N
	privInt := new(big.Int).SetBytes(privKey)

	// детерминированный k по RFC 6979
	k, err := rfc6979k(hash, privKey, n)
	if err != nil {
		return nil, err
	}

	// R = k*G
	rx, _ := curve.ScalarBaseMult(k.Bytes())
	r := new(big.Int).Mod(rx, n)
	if r.Sign() == 0 {
		return nil, errors.New("r == 0")
	}

	// S = k^-1 * (hash + r*privKey) mod n
	kInv := new(big.Int).ModInverse(k, n)
	s := new(big.Int).Mul(r, privInt)
	s.Add(s, new(big.Int).SetBytes(hash))
	s.Mul(s, kInv)
	s.Mod(s, n)

	// low-S нормализация
	halfN := new(big.Int).Rsh(n, 1)
	if s.Cmp(halfN) > 0 {
		s.Sub(n, s)
	}

	if s.Sign() == 0 {
		return nil, errors.New("s == 0")
	}

	return derEncode(r, s), nil
}

// rfc6979k генерирует детерминированный k по RFC 6979
func rfc6979k(hash, privKey []byte, n *big.Int) (*big.Int, error) {
	// упрощённая реализация: HMAC-SHA256 based
	// используем hmacSHA256
	qLen := 32
	bx := append(privKey, hash...)

	v := make([]byte, qLen)
	k := make([]byte, qLen)
	for i := range v {
		v[i] = 0x01
	}

	k = hmacSHA256(k, append(append(v, 0x00), bx...))
	v = hmacSHA256(k, v)
	k = hmacSHA256(k, append(append(v, 0x01), bx...))
	v = hmacSHA256(k, v)

	for {
		v = hmacSHA256(k, v)
		candidate := new(big.Int).SetBytes(v)
		if candidate.Sign() > 0 && candidate.Cmp(n) < 0 {
			return candidate, nil
		}
		k = hmacSHA256(k, append(v, 0x00))
		v = hmacSHA256(k, v)
	}
}

// derEncode кодирует r,s в DER формат
func derEncode(r, s *big.Int) []byte {
	rb := canonicalBytes(r)
	sb := canonicalBytes(s)
	// 0x30 <len> 0x02 <rlen> <r> 0x02 <slen> <s>
	body := make([]byte, 0, 6+len(rb)+len(sb))
	body = append(body, 0x02, byte(len(rb)))
	body = append(body, rb...)
	body = append(body, 0x02, byte(len(sb)))
	body = append(body, sb...)
	return append([]byte{0x30, byte(len(body))}, body...)
}

// canonicalBytes возвращает минимальное представление big.Int
// с ведущим 0x00 если старший бит установлен
func canonicalBytes(n *big.Int) []byte {
	b := n.Bytes()
	if len(b) > 0 && b[0]&0x80 != 0 {
		return append([]byte{0x00}, b...)
	}
	return b
}

// hash160 = RIPEMD160(SHA256(data))
func hash160(data []byte) []byte {
	h := sha256.Sum256(data)
	r := ripemd160.New()
	r.Write(h[:])
	return r.Sum(nil)
}

// --- вспомогательные функции сериализации ---

func appendUint32LE(buf []byte, v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return append(buf, b...)
}

func appendUint64LE(buf []byte, v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return append(buf, b...)
}

func appendVarInt(buf []byte, v uint64) []byte {
	switch {
	case v < 0xfd:
		return append(buf, byte(v))
	case v <= 0xffff:
		return append(buf, 0xfd, byte(v), byte(v>>8))
	case v <= 0xffffffff:
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(v))
		return append(append(buf, 0xfe), b...)
	default:
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, v)
		return append(append(buf, 0xff), b...)
	}
}

func reverseBytes(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}

// CalcFee рассчитывает комиссию в satoshi
// feePerKb — LTC/KB от Electrum, nInputs/nOutputs — количество входов/выходов
func CalcFee(feePerKb float64, nInputs, nOutputs int) int64 {
	// P2PKH: ~148 байт на input, ~34 байт на output, 10 байт overhead
	txSize := 10 + nInputs*148 + nOutputs*34
	feePerByte := feePerKb * 1e8 / 1000
	fee := int64(float64(txSize) * feePerByte)
	// минимум 1000 satoshi
	if fee < 1000 {
		fee = 1000
	}
	return fee
}

// SelectUTXOs выбирает UTXOs для покрытия суммы + комиссии
func SelectUTXOs(utxos []UTXO, targetSatoshi, feeSatoshi int64) ([]UTXO, int64, error) {
	needed := targetSatoshi + feeSatoshi
	var selected []UTXO
	var total int64
	for _, u := range utxos {
		selected = append(selected, u)
		total += u.Value
		if total >= needed {
			break
		}
	}
	if total < needed {
		return nil, 0, fmt.Errorf("недостаточно средств: нужно %.8f LTC, доступно %.8f LTC",
			float64(needed)/1e8, float64(total)/1e8)
	}
	change := total - needed
	return selected, change, nil
}

func readVarInt(data []byte, offset int) (uint64, int) {
	if offset >= len(data) {
		return 0, 0
	}
	b := data[offset]
	switch b {
	case 0xfd:
		if offset+3 > len(data) { return 0, 1 }
		return uint64(binary.LittleEndian.Uint16(data[offset+1:])), 3
	case 0xfe:
		if offset+5 > len(data) { return 0, 1 }
		return uint64(binary.LittleEndian.Uint32(data[offset+1:])), 5
	case 0xff:
		if offset+9 > len(data) { return 0, 1 }
		return binary.LittleEndian.Uint64(data[offset+1:]), 9
	default:
		return uint64(b), 1
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) { return false }
	for i := range a {
		if a[i] != b[i] { return false }
	}
	return true
}