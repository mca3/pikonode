package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/internal/config"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var ourDevice api.Device

func newDevice(ctx context.Context) (api.Device, error) {
	if config.Cfg.PrivateKey != "" {
		return api.Device{}, fmt.Errorf("refusing to make a new device with an already specified private key")
	}

	privKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return api.Device{}, fmt.Errorf("failed to generate private key: %w", err)
	}

	pubKey := privKey.PublicKey()

	config.Cfg.PublicKey = pubKey.String()
	config.Cfg.PrivateKey = privKey.String()

	/*
		wgChan <- wgMsg{
			Type: wgSetKey,
			Key:  privKey,
		}
	*/

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "<unknown>"
	}

	d, err := eng.API().NewDevice(ctx, hostname, config.Cfg.PublicKey)
	if err == nil {
		config.Cfg.DeviceID = d.ID
		if err := config.SaveConfigFile(); err != nil {
			return d, fmt.Errorf("failed to save config: %w", err)
		}
	}
	return d, err
}

func getDevice(ctx context.Context) (api.Device, error) {
	if config.Cfg.DeviceID == 0 {
		return newDevice(ctx)
	}

	dev, err := eng.API().Device(ctx, config.Cfg.DeviceID)
	if errors.Is(err, api.ErrNotFound) {
		return newDevice(ctx)
	}

	return dev, err
}
