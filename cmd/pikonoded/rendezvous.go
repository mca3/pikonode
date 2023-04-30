package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/cmd/pikonoded/wg"
	"github.com/mca3/pikonode/internal/config"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var rv *api.API
var connectedNetworks []api.Network
var peerList []api.Device
var ourDevice api.Device
var gwChan chan api.GatewayMsg
var rendezPort = 0

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

	if rendezPort == 0 {
		rendezPort = config.Cfg.ListenPort
	}

	go rv.Gateway(ctx, gwChan, dev.ID, rendezPort)
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

	return dev, err
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
	wgLock.Lock()
	defer wgLock.Unlock()

	// Find old devices
	valid := 0
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
			key, err := wg.ParseKey(d.PublicKey)
			if err != nil {
				// shouldn't happen.
				continue
			}

			log.Printf("removing peer %s", d.IP)
			wgDev.RemovePeer(key)
		} else {
			peerList[valid] = d
			valid++
		}
	}

	peerList = peerList[:valid]

	// Find new devices
	for _, v := range connectedNetworks {
		for _, d := range v.Devices {
			if d.ID == ourDevice.ID {
				continue
			}

			if !deviceIsIn(d, peerList) {
				key, err := wg.ParseKey(d.PublicKey)
				if err != nil {
					// shouldn't happen.
					continue
				}

				peerList = append(peerList, d)
				log.Printf("adding peer %s", d.IP)
				wgDev.AddPeer(mustParseIPNet(d.IP), mustParseUDPAddr(d.Endpoint), key)
			}
		}
	}
}

func getConnNw(id int64) (*api.Network, int) {
	for i, v := range connectedNetworks {
		if v.ID == id {
			return &v, i
		}
	}
	return nil, -1
}

func joinNetwork(ctx context.Context, nwid int64) {
	nw, err := rv.Network(ctx, nwid)
	if err != nil {
		log.Printf("failed to fetch networks: %v", err)
		return
	}

	connectedNetworks = append(connectedNetworks, nw)
	updatePeers()

	log.Printf("Joined network %d \"%s\"", nw.ID, nw.Name)
}

func leaveNetwork(ctx context.Context, nwid int64) {
	nw, i := getConnNw(nwid)

	if nw == nil {
		return
	}

	connectedNetworks[i], connectedNetworks[len(connectedNetworks)-1] = connectedNetworks[len(connectedNetworks)-1], connectedNetworks[i]
	connectedNetworks = connectedNetworks[:len(connectedNetworks)-1]

	updatePeers()

	log.Printf("Left network %d \"%s\"", nw.ID, nw.Name)
}

func handleJoin(ctx context.Context, dev *api.Device, nw *api.Network) {
	if dev == nil || nw == nil {
		log.Printf("received bad NetworkJoin from rendezvous")
		return
	}

	if dev.ID == ourDevice.ID {
		joinNetwork(ctx, nw.ID)
		return
	}

	cnw, _ := getConnNw(nw.ID)
	if cnw == nil {
		// We shouldn't have received this.
		log.Printf("received bad NetworkJoin from rendezvous: not part of network that device has joined")
		return
	}

	// Add to network
	cnw.Devices = append(cnw.Devices, *dev)

	updatePeers()
}

func handleLeave(ctx context.Context, dev *api.Device, nw *api.Network) {
	if dev == nil || nw == nil {
		log.Printf("received bad NetworkLeave from rendezvous")
		return
	}

	cnw, _ := getConnNw(nw.ID)
	if cnw == nil {
		// We shouldn't have received this.
		log.Printf("received bad NetworkLeave from rendezvous: not part of network that device has left")
		return
	}

	if dev.ID == ourDevice.ID {
		leaveNetwork(ctx, nw.ID)
		return
	}

	// Remove them from the network.
	for i, v := range cnw.Devices {
		if v.ID == dev.ID {
			cnw.Devices[i], cnw.Devices[len(cnw.Devices)-1] = cnw.Devices[len(cnw.Devices)-1], cnw.Devices[i]
			cnw.Devices = cnw.Devices[:len(cnw.Devices)-1]
			break
		}
	}

	updatePeers()
}

func handleUpdate(ctx context.Context, dev *api.Device) {
	wgLock.Lock()
	defer wgLock.Unlock()

	if dev == nil {
		log.Printf("received bad DeviceUpdate from rendezvous")
		return
	}

	if dev.ID == ourDevice.ID {
		ourDevice = *dev
	}

	for _, v := range connectedNetworks {
		for i, d := range v.Devices {
			if d.ID == dev.ID {
				v.Devices[i] = *dev
				break
			}
		}
	}

	if dev.ID == ourDevice.ID {
		// We aren't in our own peer list
		return
	}

	for i, d := range peerList {
		if d.ID == dev.ID {
			peerList[i] = *dev
			break
		}
	}

	key, err := wg.ParseKey(dev.PublicKey)
	if err != nil {
		log.Printf("received bad device key from rendezvous")
		return
	}

	wgDev.AddPeer(mustParseIPNet(dev.IP), mustParseUDPAddr(dev.Endpoint), key)
}

func handleGwMsg(ctx context.Context, msg api.GatewayMsg) {
	switch msg.Type {
	case api.NetworkJoin:
		handleJoin(ctx, msg.Device, msg.Network)
	case api.NetworkLeave:
		handleLeave(ctx, msg.Device, msg.Network)
	case api.DeviceUpdate:
		handleUpdate(ctx, msg.Device)
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
