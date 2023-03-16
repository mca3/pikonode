package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"time"
)

var ConfigFileOverride = ""

var Cfg = struct {
	Rendezvous    string
	Token         string
	DeviceID      int64
	PrivateKey    string
	PublicKey     string
	InterfaceName string
	ListenPort    int
}{
	Rendezvous: "http://localhost:8080/api",
	Token:      "",
	DeviceID:   0,

	InterfaceName: "pn0",
	ListenPort:    0,

	// TODO: Generate these
	PrivateKey: "",
	PublicKey:  "",
}

func resolveConfigFile() string {
	if ConfigFileOverride != "" {
		return ConfigFileOverride
	}

	Cfg, err := os.UserConfigDir()
	if err != nil {
		Cfg, err = os.Getwd()
		if err != nil {
			panic(err)
		}
	}

	return filepath.Join(Cfg, "pikonode", "config.json")
}

func SaveConfigFile() error {
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

	return json.NewEncoder(f).Encode(&Cfg)
}

func ReadConfigFile() error {
	path := resolveConfigFile()

	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		err := SaveConfigFile()
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// TODO: Sanity checks

	if Cfg.ListenPort <= 0 {
		// Pick a random one in the range 1024 <= x <= 65535
		// TODO: Validate that it is not being used
		rng := mathrand.NewSource(time.Now().Unix())
		Cfg.ListenPort = int(rng.Int63())%(65535-1024) + 1024
	}

	return json.NewDecoder(f).Decode(&Cfg)
}
