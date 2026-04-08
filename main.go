package main

import "fmt"

func main() {
	cfg, err := LoadConfig("config.json")
	if err != nil {
		cfg = DefaultConfig()
		if err := SaveConfig("config.json", cfg); err != nil {
			fmt.Println("Error saving config:", err)
		}
	}
	RunUI(cfg)
}