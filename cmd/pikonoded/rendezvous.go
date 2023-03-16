package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/internal/config"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var rv *api.API
var connectedNetworks []api.Network
var ourDevice api.Device

func createPikorv(ctx context.Context) error {
	rv = &api.API{
		Server: config.Cfg.Rendezvous,
		Token:  config.Cfg.Token,
		HTTP:   http.DefaultClient,
	}

	dev, err := getDevice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	}
	log.Printf("This device is \"%s\", ID %d", dev.Name, dev.ID)

	ourDevice = dev

	return hardRebuild(ctx)
}

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

	wgChan <- wgMsg{
		Type: wgSetKey,
		Key:  privKey,
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "<unknown>"
	}

	d, err := rv.NewDevice(ctx, hostname, config.Cfg.PublicKey)
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

	dev, err := rv.Device(ctx, config.Cfg.DeviceID)
	if errors.Is(err, api.ErrNotFound) {
		return newDevice(ctx)
	}

	return dev, nil
}

// hardRebuild repopulates connectedNetworks and ourDevice.
func hardRebuild(ctx context.Context) error {
	// Get a list of our networks
	nws, err := rv.Networks(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch networks: %v", err)
	}

	for _, v := range nws {
		ok := false
		for _, d := range v.Devices {
			if d.ID == ourDevice.ID {
				// That's us!
				ok = true
				break
			}
		}

		if ok {
			connectedNetworks = append(connectedNetworks, v)
		}
	}

	// DEBUG
	for _, v := range connectedNetworks {
		log.Printf("On network ID %d, name \"%s\", %d nodes", v.ID, v.Name, len(v.Devices))
	}

	return nil
}
