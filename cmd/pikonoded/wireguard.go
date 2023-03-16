package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"

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
	wgUp
	wgDown
)

type wgMsg struct {
	Type   wgMsgType
	IP     string
	Remove bool
	Key    string
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

// parseKey converts a base64 key into a WireGuard key.
func parseKey(key string) (wgtypes.Key, error) {
	dst := [32]byte{}

	if _, err := base64.StdEncoding.Decode(dst[:], []byte(key)); err != nil {
		return wgtypes.Key(dst), err
	}

	return wgtypes.Key(dst), nil
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

	waitGroup.Add(1)

	go func() {
		defer waitGroup.Done()
		defer netlink.LinkDel(l)
		defer wg.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case <-wgChan:
			}
		}
	}()

	return nil
}
