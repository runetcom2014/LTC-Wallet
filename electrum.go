package main

import (
	"bufio"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"
)

// ElectrumClient — клиент Electrum протокола (JSON over TCP/SSL)
type ElectrumClient struct {
	conn    net.Conn
	reader  *bufio.Reader
	mu      sync.Mutex
	idCount int
}

type electrumRequest struct {
	ID     int           `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

type electrumResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  interface{}     `json:"error"`
}

// Balance — баланс адреса в satoshi
type Balance struct {
	Confirmed   int64 `json:"confirmed"`
	Unconfirmed int64 `json:"unconfirmed"`
}

// ConnectElectrum подключается к первому доступному серверу из списка
func ConnectElectrum(servers []string) (*ElectrumClient, string, error) {
	for _, server := range servers {
		conn, err := dialElectrum(server)
		if err != nil {
			continue
		}
		client := &ElectrumClient{
			conn:   conn,
			reader: bufio.NewReader(conn),
		}
		// handshake
		if err := client.serverVersion(); err != nil {
			conn.Close()
			continue
		}
		return client, server, nil
	}
	return nil, "", fmt.Errorf("не удалось подключиться ни к одному серверу")
}

// dialElectrum подключается к серверу (SSL или TCP)
func dialElectrum(server string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(server)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(host, port)
	timeout := 10 * time.Second

	// пробуем SSL
	tlsCfg := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         host,
	}
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: timeout},
		"tcp", addr, tlsCfg,
	)
	if err == nil {
		return conn, nil
	}

	// fallback на plain TCP
	return net.DialTimeout("tcp", addr, timeout)
}

// call отправляет запрос и получает ответ
func (c *ElectrumClient) call(method string, params []interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.idCount++
	req := electrumRequest{
		ID:     c.idCount,
		Method: method,
		Params: params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	c.conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err := c.conn.Write(data); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var resp electrumResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("server error: %v", resp.Error)
	}
	return resp.Result, nil
}

// serverVersion выполняет handshake с сервером
func (c *ElectrumClient) serverVersion() error {
	_, err := c.call("server.version", []interface{}{"ltc-wallet/1.0", "1.4"})
	return err
}

// GetBalance возвращает баланс адреса
func (c *ElectrumClient) GetBalance(address string) (Balance, error) {
	scriptHash, err := addressToScriptHash(address)
	if err != nil {
		return Balance{}, fmt.Errorf("script hash: %w", err)
	}
	result, err := c.call("blockchain.scripthash.get_balance", []interface{}{scriptHash})
	if err != nil {
		return Balance{}, err
	}
	var balance Balance
	if err := json.Unmarshal(result, &balance); err != nil {
		return Balance{}, fmt.Errorf("parse balance: %w", err)
	}
	return balance, nil
}

// Close закрывает соединение
func (c *ElectrumClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// addressToScriptHash конвертирует LTC адрес в scripthash для Electrum API
// Electrum использует SHA256(script) в little-endian
// поддерживает P2PKH (L-адреса, 0x30) и P2SH (M-адреса, 0x32)
func addressToScriptHash(address string) (string, error) {
	decoded, err := base58Decode(address)
	if err != nil {
		return "", fmt.Errorf("base58 decode: %w", err)
	}
	if len(decoded) < 21 {
		return "", fmt.Errorf("invalid address length")
	}

	version := decoded[0]
	hash160 := decoded[1:21]

	var script []byte
	switch version {
	case 0x30: // P2PKH — L-адреса
		script = make([]byte, 25)
		script[0] = 0x76 // OP_DUP
		script[1] = 0xa9 // OP_HASH160
		script[2] = 0x14 // push 20 bytes
		copy(script[3:23], hash160)
		script[23] = 0x88 // OP_EQUALVERIFY
		script[24] = 0xac // OP_CHECKSIG
	case 0x32: // P2SH — M-адреса
		script = make([]byte, 23)
		script[0] = 0xa9 // OP_HASH160
		script[1] = 0x14 // push 20 bytes
		copy(script[2:22], hash160)
		script[22] = 0x87 // OP_EQUAL
	default:
		return "", fmt.Errorf("unsupported address version: 0x%02x", version)
	}

	return scriptHashFromScript(script), nil
}

// scriptHashFromScript считает SHA256(script) и реверсирует в little-endian
func scriptHashFromScript(script []byte) string {
	h := sha256.Sum256(script)
	// реверс в little-endian
	for i, j := 0, len(h)-1; i < j; i, j = i+1, j-1 {
		h[i], h[j] = h[j], h[i]
	}
	return fmt.Sprintf("%x", h)
}

// base58Decode декодирует Base58Check строку
func base58Decode(s string) ([]byte, error) {
	n := new(big.Int)
	for _, c := range s {
		idx := strings.IndexRune(base58Alphabet, c)
		if idx < 0 {
			return nil, fmt.Errorf("invalid base58 char: %c", c)
		}
		n.Mul(n, big.NewInt(58))
		n.Add(n, big.NewInt(int64(idx)))
	}
	decoded := n.Bytes()
	leadingZeros := 0
	for _, c := range s {
		if c != '1' {
			break
		}
		leadingZeros++
	}
	result := make([]byte, leadingZeros+len(decoded))
	copy(result[leadingZeros:], decoded)
	return result, nil
}

// UTXO — неизрасходованный выход транзакции
type UTXO struct {
	TxHash string `json:"tx_hash"`
	TxPos  int    `json:"tx_pos"`
	Value  int64  `json:"value"`
	Height int    `json:"height"`
}

// GetUTXOs возвращает список UTXOs для адреса
func (c *ElectrumClient) GetUTXOs(address string) ([]UTXO, error) {
	scriptHash, err := addressToScriptHash(address)
	if err != nil {
		return nil, fmt.Errorf("script hash: %w", err)
	}
	result, err := c.call("blockchain.scripthash.listunspent", []interface{}{scriptHash})
	if err != nil {
		return nil, err
	}
	var utxos []UTXO
	if err := json.Unmarshal(result, &utxos); err != nil {
		return nil, fmt.Errorf("parse utxos: %w", err)
	}
	return utxos, nil
}

// EstimateFee возвращает рекомендованную комиссию в LTC/KB для N блоков
func (c *ElectrumClient) EstimateFee(blocks int) (float64, error) {
	result, err := c.call("blockchain.estimatefee", []interface{}{blocks})
	if err != nil {
		return 0, err
	}
	var fee float64
	if err := json.Unmarshal(result, &fee); err != nil {
		return 0, fmt.Errorf("parse fee: %w", err)
	}
	if fee < 0 {
		return 0.001, nil
	}
	return fee, nil
}

// Broadcast отправляет подписанную транзакцию в сеть
func (c *ElectrumClient) Broadcast(txHex string) (string, error) {
	result, err := c.call("blockchain.transaction.broadcast", []interface{}{txHex})
	if err != nil {
		return "", err
	}
	var txid string
	if err := json.Unmarshal(result, &txid); err != nil {
		return "", fmt.Errorf("parse txid: %w", err)
	}
	// некоторые серверы добавляют предупреждения в ответ —
	// вытаскиваем только 64-символьный hex txid
	txid = extractTxID(txid)
	return txid, nil
}

// extractTxID вытаскивает 64-символьный hex txid из строки ответа сервера
func extractTxID(s string) string {
	// ищем подстроку из 64 hex символов
	hexChars := "0123456789abcdefABCDEF"
	start := -1
	count := 0
	for i, c := range s {
		if strings.ContainsRune(hexChars, c) {
			if start == -1 {
				start = i
			}
			count++
			if count == 64 {
				candidate := s[start : start+64]
				// убеждаемся что следующий символ не hex (иначе строка длиннее)
				if start+64 >= len(s) || !strings.ContainsRune(hexChars, rune(s[start+64])) {
					return strings.ToLower(candidate)
				}
			}
		} else {
			start = -1
			count = 0
		}
	}
	// если не нашли — возвращаем как есть
	return s
}

// TxHistoryItem — запись в истории транзакций
type TxHistoryItem struct {
	TxHash string `json:"tx_hash"`
	Height int    `json:"height"`
}

// GetHistory возвращает историю транзакций адреса (последние N)
func (c *ElectrumClient) GetHistory(address string, limit int) ([]TxHistoryItem, error) {
	scriptHash, err := addressToScriptHash(address)
	if err != nil {
		return nil, fmt.Errorf("script hash: %w", err)
	}
	result, err := c.call("blockchain.scripthash.get_history", []interface{}{scriptHash})
	if err != nil {
		return nil, err
	}
	var history []TxHistoryItem
	if err := json.Unmarshal(result, &history); err != nil {
		return nil, fmt.Errorf("parse history: %w", err)
	}
	// берём последние N — они в конце списка
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	// разворачиваем — новые сверху
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}
	return history, nil
}


// ClassifyTx определяет направление и сумму транзакции.
//
// Логика:
//   - Получаем raw hex транзакции (один запрос)
//   - Смотрим outputs: считаем toUs и toOthers
//   - Смотрим inputs: если хотя бы один input тратит наш адрес (prevTx → наш output),
//     значит мы отправители → исходящая, сумма = toOthers (что ушло не нам)
//   - Иначе → входящая, сумма = toUs
//
// Для проверки inputs нам нужно для каждого prevTxHash запросить его outputs[vout].
// Это O(n) запросов по числу inputs — обычно 1–3, что приемлемо.
func (c *ElectrumClient) ClassifyTx(txHash, address string) (amountStr string, incoming bool) {
	// Получаем raw hex
	result, err := c.call("blockchain.transaction.get", []interface{}{txHash, false})
	if err != nil {
		return "?", true
	}
	var txHex string
	if err := json.Unmarshal(result, &txHex); err != nil {
		return "?", true
	}

	toUs, toOthers := parseTxOutputs(txHex, address)
	inputs := parseTxInputs(txHex)

	// Проверяем: являемся ли мы отправителем (наш адрес в inputs)?
	isSender := false
	for _, inp := range inputs {
		if c.isOurOutput(inp.PrevTxHash, inp.PrevVout, address) {
			isSender = true
			break
		}
	}

	if isSender {
		// Исходящая: показываем сколько ушло не нам (за вычетом сдачи)
		return fmt.Sprintf("%.8f LTC", toOthers), false
	}
	// Входящая: показываем сколько пришло нам
	return fmt.Sprintf("%.8f LTC", toUs), true
}

// isOurOutput проверяет, принадлежит ли output[vout] нашему адресу.
func (c *ElectrumClient) isOurOutput(txHash string, vout uint32, address string) bool {
	result, err := c.call("blockchain.transaction.get", []interface{}{txHash, false})
	if err != nil {
		return false
	}
	var txHex string
	if err := json.Unmarshal(result, &txHex); err != nil {
		return false
	}
	return outputBelongsTo(txHex, vout, address)
}

// outputBelongsTo проверяет принадлежит ли output[vout] адресу.
func outputBelongsTo(txHex string, vout uint32, address string) bool {
	raw, err := hex.DecodeString(txHex)
	if err != nil || len(raw) < 10 {
		return false
	}
	decoded, err := base58Decode(address)
	if err != nil || len(decoded) < 21 {
		return false
	}
	hash160 := decoded[1:21]
	wantScript := make([]byte, 25)
	wantScript[0] = 0x76; wantScript[1] = 0xa9; wantScript[2] = 0x14
	copy(wantScript[3:23], hash160)
	wantScript[23] = 0x88; wantScript[24] = 0xac

	// пропускаем inputs
	offset := 4
	inputCount, n := readVarInt(raw, offset)
	offset += n
	for i := uint64(0); i < inputCount; i++ {
		offset += 36
		scriptLen, n := readVarInt(raw, offset)
		offset += n + int(scriptLen) + 4
		if offset > len(raw) {
			return false
		}
	}

	// читаем outputs
	outputCount, n := readVarInt(raw, offset)
	offset += n
	for i := uint64(0); i < outputCount; i++ {
		if offset+8 > len(raw) {
			break
		}
		offset += 8 // value
		scriptLen, n := readVarInt(raw, offset)
		offset += n
		if offset+int(scriptLen) > len(raw) {
			break
		}
		script := raw[offset : offset+int(scriptLen)]
		offset += int(scriptLen)
		if uint32(i) == vout {
			return len(script) == 25 && bytesEqual(script, wantScript)
		}
	}
	return false
}




// parseTxOutputs парсит outputs транзакции из hex.
// Возвращает сумму на наш адрес и сумму на другие адреса.
func parseTxOutputs(txHex, address string) (toUs float64, toOthers float64) {
	raw, err := hex.DecodeString(txHex)
	if err != nil || len(raw) < 10 {
		return
	}
	decoded, err := base58Decode(address)
	if err != nil || len(decoded) < 21 {
		return
	}
	hash160 := decoded[1:21]
	wantScript := make([]byte, 25)
	wantScript[0] = 0x76
	wantScript[1] = 0xa9
	wantScript[2] = 0x14
	copy(wantScript[3:23], hash160)
	wantScript[23] = 0x88
	wantScript[24] = 0xac

	offset := 4
	inputCount, n := readVarInt(raw, offset)
	offset += n
	for i := uint64(0); i < inputCount; i++ {
		offset += 36 // txid(32) + vout(4)
		scriptLen, n := readVarInt(raw, offset)
		offset += n + int(scriptLen) + 4 // script + sequence
		if offset > len(raw) {
			return
		}
	}
	outputCount, n := readVarInt(raw, offset)
	offset += n
	for i := uint64(0); i < outputCount; i++ {
		if offset+8 > len(raw) {
			break
		}
		valueSat := int64(binary.LittleEndian.Uint64(raw[offset : offset+8]))
		offset += 8
		scriptLen, n := readVarInt(raw, offset)
		offset += n
		if offset+int(scriptLen) > len(raw) {
			break
		}
		script := raw[offset : offset+int(scriptLen)]
		offset += int(scriptLen)
		val := float64(valueSat) / 1e8
		if len(script) == 25 && bytesEqual(script, wantScript) {
			toUs += val
		} else {
			toOthers += val
		}
	}
	return
}

func min(a, b int) int {
	if a < b { return a }
	return b
}



// TxInput — входящий input транзакции
type TxInput struct {
	PrevTxHash string
	PrevVout   uint32
}



// parseTxInputs парсит inputs из сырого hex транзакции
func parseTxInputs(txHex string) []TxInput {
	raw, err := hex.DecodeString(txHex)
	if err != nil || len(raw) < 10 {
		return nil
	}
	offset := 4
	inputCount, n := readVarInt(raw, offset)
	offset += n
	inputs := make([]TxInput, 0, inputCount)
	for i := uint64(0); i < inputCount; i++ {
		if offset+36 > len(raw) {
			break
		}
		// txid в little-endian — реверсируем
		txid := make([]byte, 32)
		copy(txid, raw[offset:offset+32])
		for l, r := 0, 31; l < r; l, r = l+1, r-1 {
			txid[l], txid[r] = txid[r], txid[l]
		}
		vout := binary.LittleEndian.Uint32(raw[offset+32 : offset+36])
		offset += 36
		scriptLen, n := readVarInt(raw, offset)
		offset += n + int(scriptLen) + 4
		if offset > len(raw) {
			break
		}
		inputs = append(inputs, TxInput{
			PrevTxHash: fmt.Sprintf("%x", txid),
			PrevVout:   vout,
		})
	}
	return inputs
}