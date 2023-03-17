package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/netip"

	"github.com/mca3/pikonode/internal/config"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var wgChan = make(chan wgMsg, 10)

type wgMsgType int

const (
	wgSetIP wgMsgType = iota
	wgSetKey
	wgPeer
)

type wgMsg struct {
	Type     wgMsgType
	IP       string
	Endpoint string
	Remove   bool
	Key      wgtypes.Key
}

// nlWireguard implements netlink.Link, as there is no native way to do this in
// the netlink package yet.
type nlWireguard struct {
	netlink.LinkAttrs
}

func (w *nlWireguard) Attrs() *netlink.LinkAttrs {
	return &w.LinkAttrs
}

func (w *nlWireguard) Type() string {
	return "wireguard"
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

func handleWgMsg(link netlink.Link, wg *wgctrl.Client, msg wgMsg) {
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
		err = netlink.AddrAdd(link, &netlink.Addr{IPNet: ipn})
	case wgSetKey:
		log.Printf("setting WireGuard key")

		err = wg.ConfigureDevice(link.Attrs().Name, wgtypes.Config{
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
			rmRoute(link, ipn)
		} else {
			ep := msg.UDP() // Could be nil
			peer.Endpoint = ep
			peer.AllowedIPs = []net.IPNet{*ipn}

			log.Printf("Adding %s as WireGuard peer (Endpoint: %v)", ipn.IP.String(), ep)
			addRoute(link, ipn)
		}

		err = wg.ConfigureDevice(link.Attrs().Name, wgtypes.Config{
			Peers: []wgtypes.PeerConfig{peer},
		})
	}

	if err != nil {
		log.Printf("couldn't handle wireguard request %v: %v", msg.Type, err)
	}
}

func goWireguard(ctx context.Context, link netlink.Link, wg *wgctrl.Client) {
	defer waitGroup.Done()
	defer netlink.LinkDel(link)
	defer wg.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case e := <-wgChan:
			handleWgMsg(link, wg, e)
		}
	}
}

func createWireguard(ctx context.Context) error {
	// Create the interface
	attrs := netlink.NewLinkAttrs()
	attrs.Name = config.Cfg.InterfaceName
	wga := nlWireguard{attrs}

	err := netlink.LinkAdd(&wga)
	if err != nil {
		return err
	}

	l, err := netlink.LinkByName(attrs.Name)
	if err != nil {
		// This shouldn't fail...
		log.Fatalf("cannot access the interface (%s) we just made: %v", attrs.Name, err)
	}

	// Unfortunately since this spawns a goroutine to handle messages being
	// passed to it since I'm not entirely sure if wgctrl-go will like
	// multiple goroutines using it at once, we cannot defer and must
	// instead cleanup whenever we would exit in a bad way.

	// Configure WireGuard
	wg, err := wgctrl.New()
	if err != nil {
		netlink.LinkDel(l)
		return err
	}

	wgkey, err := parseKey(config.Cfg.PrivateKey)
	if err != nil {
		wg.Close()
		netlink.LinkDel(l)
		return fmt.Errorf("couldn't parse private key: %w", err)
	}

	if err := wg.ConfigureDevice(attrs.Name, wgtypes.Config{
		PrivateKey: &wgkey,
		ListenPort: &config.Cfg.ListenPort,
	}); err != nil {
		wg.Close()
		netlink.LinkDel(l)
		return err
	}

	// Set up
	if err := netlink.LinkSetUp(l); err != nil {
		wg.Close()
		netlink.LinkDel(l)
		return err
	}

	waitGroup.Add(1)

	go goWireguard(ctx, l, wg)

	return nil
}

func addRoute(link netlink.Link, addr *net.IPNet) {
	if err := netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Protocol:  6,
		Dst:       addr,
	}); err != nil {
		log.Printf("failed to add route for %s: %v", addr.IP, err)
	}
}

func rmRoute(link netlink.Link, addr *net.IPNet) {
	routes, err := netlink.RouteGet(addr.IP)
	if err != nil {
		return
	}

	for _, v := range routes {
		if v.LinkIndex == link.Attrs().Index {
			netlink.RouteDel(&v)
			break
		}
	}
}
