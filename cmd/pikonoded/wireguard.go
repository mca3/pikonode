package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/netip"

	"github.com/mca3/pikonode/cmd/pikonoded/ifctl"
	"github.com/mca3/pikonode/internal/config"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var wgChan = make(chan wgMsg, 10)
var wgIsUp bool // this variable is racy

type wgMsgType int

const (
	wgSetIP wgMsgType = iota
	wgSetKey
	wgPeer
	wgDown
	wgUp
)

type wgMsg struct {
	Type     wgMsgType
	IP       string
	Endpoint string
	Remove   bool
	Key      wgtypes.Key
}

func (m *wgMsg) UDP() *net.UDPAddr {
	if m.Endpoint == "" {
		return nil
	}

	ap, err := netip.ParseAddrPort(m.Endpoint)
	if err != nil {
		return nil
	}

	return net.UDPAddrFromAddrPort(ap)
}

func (m *wgMsg) IPNet() *net.IPNet {
	_, ipn, err := net.ParseCIDR(m.IP + "/128")
	if err != nil {
		return nil
	}
	return ipn
}

// parseKey converts a base64 key into a WireGuard key.
func parseKey(key string) (wgtypes.Key, error) {
	dst := [32]byte{}

	if _, err := base64.StdEncoding.Decode(dst[:], []byte(key)); err != nil {
		return wgtypes.Key(dst), err
	}

	return wgtypes.Key(dst), nil
}

func handleWgMsg(ifc ifctl.Interface, wg *wgctrl.Client, msg wgMsg) {
	var err error
	switch msg.Type {
	case wgSetIP:
		log.Printf("setting WireGuard IP to %s", msg.IP)

		var ipn *net.IPNet
		_, ipn, err = net.ParseCIDR(msg.IP + "/128")
		if err != nil {
			break
		}

		// Tell the kernel where we live.
		err = ifc.SetAddr(ipn)
	case wgSetKey:
		log.Printf("setting WireGuard key")

		err = wg.ConfigureDevice(config.Cfg.InterfaceName, wgtypes.Config{
			PrivateKey: &msg.Key,
		})
	case wgPeer:
		peer := wgtypes.PeerConfig{
			PublicKey: msg.Key,
		}
		ipn := msg.IPNet() // Never nil

		if msg.Remove {
			peer.Remove = true

			log.Printf("Removing %s as WireGuard peer", ipn.IP.String())
			ifc.DeleteRoute(ipn)
		} else {
			ep := msg.UDP() // Could be nil
			peer.Endpoint = ep
			peer.AllowedIPs = []net.IPNet{*ipn}

			log.Printf("Adding %s as WireGuard peer (Endpoint: %v)", ipn.IP.String(), ep)
			ifc.AddRoute(ipn)
		}

		err = wg.ConfigureDevice(config.Cfg.InterfaceName, wgtypes.Config{
			Peers: []wgtypes.PeerConfig{peer},
		})
	case wgUp:
		if wgIsUp {
			return
		}

		err = ifc.Set(true)
		wgIsUp = true
	case wgDown:
		if !wgIsUp {
			return
		}

		err = ifc.Set(false)
		wgIsUp = false
	}

	if err != nil {
		log.Printf("couldn't handle wireguard request %v: %v", msg.Type, err)
	}
}

func goWireguard(ctx context.Context, ifc ifctl.Interface, wg *wgctrl.Client) {
	defer waitGroup.Done()
	defer ifc.Delete()
	defer wg.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case e := <-wgChan:
			handleWgMsg(ifc, wg, e)
		}
	}
}

func createWireguard(ctx context.Context) error {
	// Create the interface
	l, err := ifctl.New(config.Cfg.InterfaceName)

	// Unfortunately since this spawns a goroutine to handle messages being
	// passed to it since I'm not entirely sure if wgctrl-go will like
	// multiple goroutines using it at once, we cannot defer and must
	// instead cleanup whenever we would exit in a bad way.

	// Configure WireGuard
	wg, err := wgctrl.New()
	if err != nil {
		l.Delete()
		return err
	}

	wgkey, err := parseKey(config.Cfg.PrivateKey)
	if err != nil {
		wg.Close()
		l.Delete()
		return fmt.Errorf("couldn't parse private key: %w", err)
	}

	if err := wg.ConfigureDevice(config.Cfg.InterfaceName, wgtypes.Config{
		PrivateKey: &wgkey,
		ListenPort: &config.Cfg.ListenPort,
	}); err != nil {
		wg.Close()
		l.Delete()
		return err
	}

	waitGroup.Add(1)

	go goWireguard(ctx, l, wg)

	return nil
}
