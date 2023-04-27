package wg

import (
	"encoding/base64"
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Device implements an interface to a WireGuard device, whether it be an
// in-kernel implementation or based upon wireguard-go.
//
// A Device is not thread-safe.
// Always use a mutex when using a device from multiple goroutines.
type Device interface {
	// SetKey sets the private key of the WireGuard interface.
	SetKey(privateKey wgtypes.Key) error

	// SetListenPort sets the listening port of the WireGuard interface.
	SetListenPort(port uint16) error

	// SetIP sets the IP of the WireGuard interface.
	SetIP(newIP *net.IPNet) error

	// SetState sets the state of the WireGuard interface to "up" (true) or
	// "down" (false).
	//
	// A "down" WireGuard interface will be unable to handle any traffic.
	SetState(up bool) error

	// AddPeer adds a new peer or updates existing peer information.
	//
	// A Peer already exists when a peer with the same key as publicKey has
	// been previously added to the interface.
	//
	// endpoint may be nil, and ip may be empty, but not at the same time.
	// publicKey must always be specified.
	AddPeer(ip *net.IPNet, endpoint *net.UDPAddr, publicKey wgtypes.Key) error

	// RemovePeer disconnects from the specified peer by their public key.
	//
	// If the public key is not found in an existing peer, this function
	// does nothing.
	RemovePeer(publicKey wgtypes.Key) error

	// Close closes the WireGuard interface and cleans up.
	Close() error
}

var wgKeepalive = time.Second * 20

// ParseKey converts a base64 key into a WireGuard key.
func ParseKey(key string) (wgtypes.Key, error) {
	dst := [32]byte{}

	if _, err := base64.StdEncoding.Decode(dst[:], []byte(key)); err != nil {
		return wgtypes.Key(dst), err
	}

	return wgtypes.Key(dst), nil
}

// New creates a new WireGuard device.
func New(name string) (Device, error) {
	return newTUNWireguard(name)
	// return newNativeWireguard(name)
}
