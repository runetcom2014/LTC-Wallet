package main

/*
#cgo LDFLAGS: -L. ritual.dll
#include <stdlib.h>

extern int   RitualNew();
extern void  RitualFree(int h);
extern void  RitualFreeString(char* s);
extern char* RitualAddRite(int h, char* name);
extern char* RitualUpdateRite(int h, int id, char* payloadJSON);
extern char* RitualRemoveRite(int h, int id);
extern char* RitualGetRitePayload(int h, int id);
extern char* RitualFinalize(int h);
extern char* RitualGetState(int h);
extern char* RitualGetEntropy(int h);
extern char* RitualGetRiteDataset(char* name);
*/
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"
)

// Ritual — обёртка над ritual.dll
type Ritual struct {
	handle C.int
}

// RiteInfo описывает один обряд в ритуале
type RiteInfo struct {
	ID      int    `json:"id"`
	Type    string `json:"type"`
	HasData bool   `json:"hasData"`
}

// RitualState — текущее состояние ритуала
type RitualState struct {
	Rites []RiteInfo `json:"rites"`
}

// EntropyState — энтропия по обрядам и суммарная
type RiteEntropy struct {
	ID   int     `json:"id"`
	Bits float64 `json:"bits"`
}

type EntropyState struct {
	Rites []RiteEntropy `json:"rites"`
	Total float64       `json:"total"`
}

// FinalizeResult — результат финализации ритуала
type FinalizeResult struct {
	Key       string  `json:"key"`
	TotalBits float64 `json:"totalBits"`
	Error     string  `json:"error,omitempty"`
}

// gostr конвертирует C строку в Go строку и освобождает память
func gostr(cs *C.char) string {
	s := C.GoString(cs)
	C.RitualFreeString(cs)
	return s
}

// parseError проверяет JSON ответ на наличие поля error
func parseError(s string) error {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return nil
	}
	if errMsg, ok := result["error"].(string); ok {
		return errors.New(errMsg)
	}
	return nil
}

// NewRitual создаёт новый ритуал
func NewRitual() *Ritual {
	return &Ritual{handle: C.RitualNew()}
}

// Free освобождает ресурсы ритуала
func (r *Ritual) Free() {
	C.RitualFree(r.handle)
}

// AddRite добавляет обряд по имени типа (STRING, FILE, SEQUENCE, RUNEGRID, CONSTELLATION, CITYTIME)
// Возвращает ID обряда
func (r *Ritual) AddRite(name string) (int, error) {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	result := gostr(C.RitualAddRite(r.handle, cs))
	if err := parseError(result); err != nil {
		return 0, err
	}
	var resp map[string]int
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return 0, fmt.Errorf("addRite parse: %w", err)
	}
	return resp["id"], nil
}

// UpdateRite обновляет payload обряда, возвращает текущую энтропию
func (r *Ritual) UpdateRite(id int, payload []interface{}) (float64, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("payload marshal: %w", err)
	}
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	result := gostr(C.RitualUpdateRite(r.handle, C.int(id), cs))
	if err := parseError(result); err != nil {
		return 0, err
	}
	var resp struct {
		TotalBits float64 `json:"TotalBits"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return 0, fmt.Errorf("updateRite parse: %w", err)
	}
	return resp.TotalBits, nil
}

// RemoveRite удаляет обряд по ID
func (r *Ritual) RemoveRite(id int) error {
	result := gostr(C.RitualRemoveRite(r.handle, C.int(id)))
	return parseError(result)
}

// GetState возвращает текущее состояние ритуала
func (r *Ritual) GetState() (RitualState, error) {
	result := gostr(C.RitualGetState(r.handle))
	var state RitualState
	if err := json.Unmarshal([]byte(result), &state); err != nil {
		return RitualState{}, fmt.Errorf("getState parse: %w", err)
	}
	return state, nil
}

// GetEntropy возвращает энтропию по обрядам и суммарную
func (r *Ritual) GetEntropy() (EntropyState, error) {
	result := gostr(C.RitualGetEntropy(r.handle))
	var state EntropyState
	if err := json.Unmarshal([]byte(result), &state); err != nil {
		return EntropyState{}, fmt.Errorf("getEntropy parse: %w", err)
	}
	return state, nil
}

// Finalize завершает ритуал и возвращает 32-байтовый мастер ключ
func (r *Ritual) Finalize() ([32]byte, float64, error) {
	result := gostr(C.RitualFinalize(r.handle))
	if err := parseError(result); err != nil {
		return [32]byte{}, 0, err
	}
	var resp FinalizeResult
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		return [32]byte{}, 0, fmt.Errorf("finalize parse: %w", err)
	}
	var key [32]byte
	keyBytes, err := hexDecode(resp.Key)
	if err != nil {
		return [32]byte{}, 0, fmt.Errorf("key decode: %w", err)
	}
	copy(key[:], keyBytes)
	return key, resp.TotalBits, nil
}

// GetCityList возвращает список городов из датасета CITYTIME
func GetCityList() []string {
	cs := C.CString("CITYTIME")
	defer C.free(unsafe.Pointer(cs))
	result := gostr(C.RitualGetRiteDataset(cs))

	var data struct {
		Cities []string `json:"cities"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return nil
	}
	return data.Cities
}

// SequenceDataset — данные для SEQUENCE обряда
type SequenceDataset struct {
	Symbols []string          `json:"symbols"`
	Emoji   map[string]string `json:"emoji"`
}

// GetSequenceDataset возвращает датасет для SEQUENCE
func GetSequenceDataset() SequenceDataset {
	cs := C.CString("SEQUENCE")
	defer C.free(unsafe.Pointer(cs))
	result := gostr(C.RitualGetRiteDataset(cs))
	var data SequenceDataset
	json.Unmarshal([]byte(result), &data)
	return data
}

// RuneDataset — данные для RUNEGRID обряда
type RuneDataset struct {
	Runes        []string `json:"runes"`
	RuneNames    []string `json:"runeNames"`
	RuneSVGPaths []string `json:"runeSVGs"`
	GridSize     int      `json:"gridSize"`
	GridCount    int      `json:"gridCount"`
}

// GetRuneDataset возвращает датасет для RUNEGRID
func GetRuneDataset() RuneDataset {
	cs := C.CString("RUNEGRID")
	defer C.free(unsafe.Pointer(cs))
	result := gostr(C.RitualGetRiteDataset(cs))
	var data RuneDataset
	json.Unmarshal([]byte(result), &data)
	return data
}

// StarData — одна звезда для CONSTELLATION
type StarData struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Size  string  `json:"size"`
	Color string  `json:"color"`
	Name  string  `json:"name"`
}

// ConstellationDataset — данные для CONSTELLATION обряда
type ConstellationDataset struct {
	Stars []StarData `json:"stars"`
	Steps int        `json:"steps"`
}

// GetConstellationDataset возвращает датасет для CONSTELLATION
func GetConstellationDataset() ConstellationDataset {
	cs := C.CString("CONSTELLATION")
	defer C.free(unsafe.Pointer(cs))
	result := gostr(C.RitualGetRiteDataset(cs))
	var data ConstellationDataset
	json.Unmarshal([]byte(result), &data)
	return data
}

// hexDecode декодирует hex строку в байты
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errors.New("invalid hex length")
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		var v byte
		for _, c := range s[i : i+2] {
			v <<= 4
			switch {
			case c >= '0' && c <= '9':
				v |= byte(c - '0')
			case c >= 'a' && c <= 'f':
				v |= byte(c-'a') + 10
			case c >= 'A' && c <= 'F':
				v |= byte(c-'A') + 10
			default:
				return nil, fmt.Errorf("invalid hex char: %c", c)
			}
		}
		b[i/2] = v
	}
	return b, nil
}