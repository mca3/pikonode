package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var configFileOverride = ""

var cfg = struct {
	Rendezvous string
	Token      string
	DeviceID   int64
	PrivateKey string
	PublicKey  string
}{
	Rendezvous: "http://localhost:8080/api",
	Token:      "",
	DeviceID:   0,

	// TODO: Generate these
	PrivateKey: "",
	PublicKey:  "",
}

func resolveConfigFile() string {
	if configFileOverride != "" {
		return configFileOverride
	}

	cfg, err := os.UserConfigDir()
	if err != nil {
		cfg, err = os.Getwd()
		if err != nil {
			panic(err)
		}
	}

	return filepath.Join(cfg, "pikonode", "config.json")
}

func saveConfigFile() error {
	// TODO: Generate keys if needed

	path := resolveConfigFile()

	base := filepath.Dir(path)
	if err := os.MkdirAll(base, 0o777); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(&cfg)
}

func readConfigFile() error {
	path := resolveConfigFile()

	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		err := saveConfigFile()
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// TODO: Sanity checks

	return json.NewDecoder(f).Decode(&cfg)
}
