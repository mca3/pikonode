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
var peerList []api.Device
var ourDevice api.Device
var gwChan chan api.GatewayMsg

func createPikorv(ctx context.Context) error {
	rv = &api.API{
		Server: config.Cfg.Rendezvous,
		Token:  config.Cfg.Token,
		HTTP:   http.DefaultClient,
	}

	gwChan = make(chan api.GatewayMsg, 100)

	dev, err := getDevice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	}
	log.Printf("This device is \"%s\", ID %d", dev.Name, dev.ID)

	ourDevice = dev

	go rv.Gateway(ctx, gwChan, dev.ID, config.Cfg.ListenPort)
	go handleGateway(ctx)

	return nil
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

// hardRebuild repopulates connectedNetworks.
func hardRebuild(ctx context.Context) error {
	connectedNetworks = connectedNetworks[:0]

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

func deviceIsIn(needle api.Device, haystack []api.Device) bool {
	for _, v := range haystack {
		if needle.ID == v.ID {
			return true
		}
	}
	return false
}

// updatePeers tells WireGuard about added or removed peers.
func updatePeers() {
	// Find old devices
	for _, d := range peerList {
		if d.ID == ourDevice.ID {
			continue
		}

		ok := false
		for _, v := range connectedNetworks {
			if deviceIsIn(d, v.Devices) {
				ok = true
				break
			}
		}

		if !ok {
			// Must be removed
			key, err := parseKey(d.PublicKey)
			if err != nil {
				// shouldn't happen.
				continue
			}

			wgChan <- wgMsg{
				Type:   wgPeer,
				Remove: true,
				Key:    key,
			}
		}
	}

	// Find new devices
	for _, v := range connectedNetworks {
		for _, d := range v.Devices {
			if d.ID == ourDevice.ID {
				continue
			}

			if !deviceIsIn(d, peerList) {
				key, err := parseKey(d.PublicKey)
				if err != nil {
					// shouldn't happen.
					continue
				}

				peerList = append(peerList, d)
				wgChan <- wgMsg{
					Type:     wgPeer,
					IP:       d.IP,
					Endpoint: d.Endpoint,
					Key:      key,
				}
			}
		}
	}
}

func handleGwMsg(ctx context.Context, msg api.GatewayMsg) {
	switch msg.Type {
	case api.Peer:
		key, err := parseKey(msg.Device.PublicKey)
		if err != nil {
			// shouldn't happen.
			return
		}

		wgChan <- wgMsg{
			Type:     wgPeer,
			Remove:   msg.Remove,
			Key:      key,
			Endpoint: msg.Device.Endpoint,
			IP:       msg.Device.IP,
		}
	case api.Connect:
		log.Printf("Connected to rendezvous server")
		if hardRebuild(ctx) != nil {
			return
		}

		updatePeers()
		log.Printf("State synchronized")
	case api.Disconnect:
		log.Printf("Disconnected from rendezvous server. Error: %v", msg.Error)
		log.Printf("Reconnecting to rendezvous in %v", msg.Delay)
	}
}

func handleGateway(ctx context.Context) {
	for {
		select {
		case v := <-gwChan:
			handleGwMsg(ctx, v)
		case <-ctx.Done():
			return
		}
	}
}
