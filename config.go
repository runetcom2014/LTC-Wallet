package main

import (
	"encoding/json"
	"os"
)

// Config — конфигурация кошелька
type Config struct {
	Coin    string              `json:"coin"`
	Servers map[string][]string `json:"servers"`
}

// LoadConfig загружает конфиг из файла
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// DefaultConfig возвращает конфиг по умолчанию
func DefaultConfig() Config {
	return Config{
		Coin: "LTC",
		Servers: map[string][]string{
			"LTC": {
				"ltc-electrum.cakewallet.com:50002",
				"0xrpc.io:60002",
				"electrum-ltc.petrkr.net:60002",
				"electrum2.cipig.net:20063",
				"ltc.aftrek.org:50002",
				"5.78.97.174:50002",
			},
		},
	}
}

// SaveConfig сохраняет конфиг в файл
func SaveConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
