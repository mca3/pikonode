package main

import (
	"log"
	"sync"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/internal/config"
	"github.com/mca3/pikonode/net/wg"
)

var wgDev wg.Device
var wgLock sync.Mutex
var wgLastPeers []api.Device

// startWireguard creates the WireGuard interface.
func startWireguard() error {
	wgLock.Lock()
	defer wgLock.Unlock()

	key, err := wg.ParseKey(config.Cfg.PrivateKey)
	if err != nil {
		return err
	}

	wgDev, err = wg.New(config.Cfg.InterfaceName)
	if err != nil {
		return err
	}

	if err := wgDev.SetKey(key); err != nil {
		wgDev.Close()
		return err
	}

	if err := wgDev.SetListenPort(uint16(config.Cfg.ListenPort)); err != nil {
		wgDev.Close()
		return err
	}

	log.Printf("WireGuard interface is %v. Listen port is %d.", config.Cfg.InterfaceName, config.Cfg.ListenPort)

	return nil
}

func wgOnJoin(nw *api.Network, dev *api.Device) {
	wgUpdatePeers()
}

func wgOnLeave(nw *api.Network, dev *api.Device) {
	wgUpdatePeers()
}

func wgOnUpdate(dev *api.Device) {
	wgLock.Lock()
	defer wgLock.Unlock()

	// Largely TODO

	key, err := wg.ParseKey(dev.PublicKey)
	if err != nil {
		// shouldn't happen.
		return
	}

	wgDev.AddPeer(mustParseIPNet(dev.IP), mustParseUDPAddr(dev.Endpoint), key)
}

func wgOnRebuild() {
	wgDev.SetIP(mustParseIPNet(eng.Self().IP))
	wgUpdatePeers()
}

// indexDevice returns the index of a device in a slice, or -1 otherwise.
func indexDevice(dev api.Device, l []api.Device) int {
	for i, v := range l {
		if v.ID == dev.ID {
			return i
		}
	}

	return -1
}

func wgUpdatePeers() {
	wgLock.Lock()
	defer wgLock.Unlock()

	peers := eng.Peers()

	// Find old devices
	for _, v := range wgLastPeers {
		devIndex := indexDevice(v, peers)
		if devIndex != -1 {
			// TODO: Determine if we should update them
			continue
		}

		// Lost device
		key, err := wg.ParseKey(v.PublicKey)
		if err != nil {
			// shouldn't happen.
			continue
		}

		log.Printf("removing peer %s", v.IP)
		wgDev.RemovePeer(key)
	}

	// Find new devices
	for _, v := range peers {
		devIndex := indexDevice(v, wgLastPeers)
		if devIndex != -1 {
			continue
		}

		// New device
		key, err := wg.ParseKey(v.PublicKey)
		if err != nil {
			// shouldn't happen.
			continue
		}

		log.Printf("adding peer %s", v.IP)
		wgDev.AddPeer(mustParseIPNet(v.IP), mustParseUDPAddr(v.Endpoint), key)
	}

	// Copy new peer list
	wgLastPeers = wgLastPeers[:0]
	wgLastPeers = append(wgLastPeers, peers...)
}
