package wg

import (
	"net"

	"github.com/mca3/pikonode/cmd/pikonoded/ifctl"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// nativeWireguard implements an interface to the in-kernel implementation of
// WireGuard, using wgctrl.
type nativeWireguard struct {
	ifc ifctl.Interface
	ifn string
	wgc *wgctrl.Client
}

var (
	_ Device = &nativeWireguard{}
)

// newNativeWireguard attempts to create a new native Wireguard connection.
func newNativeWireguard(name string) (Device, error) {
	// Create the interface
	l, err := ifctl.New(name)
	if err != nil {
		return nil, err
	}

	// Configure WireGuard
	wgc, err := wgctrl.New()
	if err != nil {
		l.Delete()
		return nil, err
	}

	nwg := &nativeWireguard{
		ifc: l,
		ifn: name,
		wgc: wgc,
	}

	return nwg, nil
}

// SetKey sets the private key of the WireGuard interface.
func (w *nativeWireguard) SetKey(privateKey wgtypes.Key) error {
	return w.wgc.ConfigureDevice(w.ifn, wgtypes.Config{
		PrivateKey: &privateKey,
	})
}

// SetListenPort sets the listening port of the WireGuard interface.
func (w *nativeWireguard) SetListenPort(port uint16) error {
	iPort := int(port)

	return w.wgc.ConfigureDevice(w.ifn, wgtypes.Config{
		ListenPort: &iPort,
	})
}

// SetIP sets the IP of the WireGuard interface.
func (w *nativeWireguard) SetIP(newIP *net.IPNet) error {
	return w.ifc.SetAddr(newIP)
}

// SetState sets the state of the WireGuard interface to "up" (true) or
// "down" (false).
//
// A "down" WireGuard interface will be unable to handle any traffic.
func (w *nativeWireguard) SetState(up bool) error {
	return w.ifc.Set(up)
}

// AddPeer adds a new peer or updates existing peer information.
//
// A Peer already exists when a peer with the same key as publicKey has
// been previously added to the interface.
//
// endpoint may be nil, and ip may be empty, but not at the same time.
// publicKey must always be specified.
func (w *nativeWireguard) AddPeer(ip *net.IPNet, endpoint *net.UDPAddr, publicKey wgtypes.Key) error {
	peer := wgtypes.PeerConfig{
		PublicKey:                   publicKey,
		PersistentKeepaliveInterval: &wgKeepalive,
	}

	if endpoint != nil {
		peer.Endpoint = endpoint
	}

	if ip != nil {
		peer.AllowedIPs = []net.IPNet{*ip}

		if err := w.ifc.AddRoute(ip); err != nil {
			return err
		}
	}

	return w.wgc.ConfigureDevice(w.ifn, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peer},
	})
}

// RemovePeer disconnects from the specified peer by their public key.
//
// If the public key is not found in an existing peer, this function
// does nothing.
func (w *nativeWireguard) RemovePeer(publicKey wgtypes.Key) error {
	peer := wgtypes.PeerConfig{
		PublicKey: publicKey,
		Remove:    true,
	}

	// TODO: Should be remove the route?
	// Previously we would.

	return w.wgc.ConfigureDevice(w.ifn, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peer},
	})
}

// Close closes the WireGuard interface and cleans up.
func (w *nativeWireguard) Close() error {
	if err := w.wgc.Close(); err != nil {
		return err
	}
	if err := w.ifc.Delete(); err != nil {
		return err
	}

	return nil
}
