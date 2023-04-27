package wg

import (
	"net"
	"log"
	"fmt"

	"github.com/mca3/pikonode/cmd/pikonoded/ifctl"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// wgctrlWireguard implements an interface to the in-kernel implementation of
// WireGuard, using wgctrl.
type wgctrlWireguard struct {
	ifc ifctl.Interface
	ifn string
	wgc *wgctrl.Client
}

var (
	_ Device = &wgctrlWireguard{}
)

// newNativeWireguard attempts to create a new wgctrl Wireguard connection
// using the in-kernel module.
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

	nwg := &wgctrlWireguard{
		ifc: l,
		ifn: name,
		wgc: wgc,
	}

	return nwg, nil
}

// newTUNWireguard creates a new TUN device for use with Wireguard.
func newTUNWireguard(name string) (Device, error) {
	// Setup TUN adapter using wireguard-go
	// This is long and convoluted.
	t, err := tun.CreateTUN(name, device.DefaultMTU)
	if err != nil {
		return nil, err
	}

	il, err := uapiOpen(name)
	if err != nil {
		t.Close()
		return nil, err
	}

	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("(%s) ", name))

	dev := device.NewDevice(t, conn.NewDefaultBind(), logger)

	uapi, err := uapiListen(name, il)
	if err != nil {
		dev.Close()
		il.Close()
		t.Close()
		return nil, err
	}

	go func() {
		defer t.Close()
		defer il.Close()
		defer dev.Close()
		defer uapi.Close()	

		for {
			conn, err := uapi.Accept()
			if err != nil {
				log.Fatal(err)
			}
			go dev.IpcHandle(conn)
		}
	}()

	// Grab the interface
	realName, err := t.Name()
	if err != nil {
		realName = name
	}

	ifc, err := ifctl.From(realName)
	if err != nil {
		uapi.Close()
		dev.Close()
		il.Close()
		t.Close()
	}

	// Configure WireGuard
	wgc, err := wgctrl.New()
	if err != nil {
		uapi.Close()
		dev.Close()
		il.Close()
		t.Close()
		return nil, err
	}

	nwg := &wgctrlWireguard{
		ifc: ifc,
		ifn: name,
		wgc: wgc,
	}

	return nwg, nil
}

// SetKey sets the private key of the WireGuard interface.
func (w *wgctrlWireguard) SetKey(privateKey wgtypes.Key) error {
	return w.wgc.ConfigureDevice(w.ifn, wgtypes.Config{
		PrivateKey: &privateKey,
	})
}

// SetListenPort sets the listening port of the WireGuard interface.
func (w *wgctrlWireguard) SetListenPort(port uint16) error {
	iPort := int(port)

	return w.wgc.ConfigureDevice(w.ifn, wgtypes.Config{
		ListenPort: &iPort,
	})
}

// SetIP sets the IP of the WireGuard interface.
func (w *wgctrlWireguard) SetIP(newIP *net.IPNet) error {
	return w.ifc.SetAddr(newIP)
}

// SetState sets the state of the WireGuard interface to "up" (true) or
// "down" (false).
//
// A "down" WireGuard interface will be unable to handle any traffic.
func (w *wgctrlWireguard) SetState(up bool) error {
	return w.ifc.Set(up)
}

// AddPeer adds a new peer or updates existing peer information.
//
// A Peer already exists when a peer with the same key as publicKey has
// been previously added to the interface.
//
// endpoint may be nil, and ip may be empty, but not at the same time.
// publicKey must always be specified.
func (w *wgctrlWireguard) AddPeer(ip *net.IPNet, endpoint *net.UDPAddr, publicKey wgtypes.Key) error {
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
func (w *wgctrlWireguard) RemovePeer(publicKey wgtypes.Key) error {
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
func (w *wgctrlWireguard) Close() error {
	if err := w.wgc.Close(); err != nil {
		return err
	}
	if err := w.ifc.Delete(); err != nil {
		return err
	}

	return nil
}
