package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/internal/config"
	"github.com/mca3/pikonode/net/wg"
	"github.com/mca3/pikonode/piko"
)

// All of the waitGroup tomfoolery is because we have stuff to clean up on exit
// via defers and when main() exits, the defers on our couple of goroutines do
// not run.
// This is likely not an ideal solution.
var waitGroup = sync.WaitGroup{}

var eng *piko.Engine

// getRuntimeDir gets the runtime directory of the current user, or uses the
// current working directory as a fallback.
func getRuntimeDir() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		log.Printf("XDG_RUNTIME_DIR is unset; using cwd")

		var err error
		runtimeDir, err = os.Getwd()
		if err != nil {
			log.Fatalf("failed to get working directory: %v", err)
		}
	}

	return runtimeDir
}

func updateAddr(ctx context.Context, pd api.PunchDetails) error {
	addr, err := fetchEndpoint(ctx, fmt.Sprintf("[%s]:8743", pd.IP))
	if err != nil {
		return fmt.Errorf("failed to contact pikopunch: %v", err)
	}

	return eng.API().GatewaySend(ctx, api.GatewayMsg{
		Type:     api.Ping,
		DeviceID: ourDevice.ID,
		Endpoint: addr,
	})
}

func mustParseIPNet(ip string) *net.IPNet {
	_, ipn, err := net.ParseCIDR(ip + "/128")
	if err != nil {
		panic(err)
	}
	return ipn
}

func mustParseUDPAddr(ip string) *net.UDPAddr {
	if ip == "" {
		return nil
	}

	ap := netip.MustParseAddrPort(ip)
	return net.UDPAddrFromAddrPort(ap)
}

// startPikopunch starts the pikopunch client.
func startPikopunch(ctx context.Context) error {
	wgLock.Lock()
	defer wgLock.Unlock()

	// Request and parse pikopunch details.
	pd, err := eng.API().PunchDetails(ctx)
	if err != nil {
		return fmt.Errorf("failed to request pikopunch details: %w", err)
	}

	pdkey, err := wg.ParseKey(pd.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse pikopunch key: %w", err)
	}

	// Add it as a peer to Wireguard, as we work over Wireguard.
	if err := wgDev.AddPeer(mustParseIPNet(pd.IP), mustParseUDPAddr(pd.Endpoint), pdkey); err != nil {
		return fmt.Errorf("failed to add pikopunch peer: %w", err)
	}

	go func() {
		// The goal of this goroutine is to occasionally talk to the
		// server and ask what our endpoint is to them, which we then
		// report back to the gateway which *may* tell others.
		//
		// We ping pikopunch every 20 seconds, which would serve as a
		// keepalive short enough that we wouldn't need to worry about
		// the port closing.
		//
		// Note that on most networks it's probably longer of a
		// timeout, but this is for the most aggressive ones,
		// hopefully.

		select {
		case <-time.After(time.Second * 2):
			// Wait for WireGuard to settle
		case <-ctx.Done():
			return
		}

		// Update our address once WireGuard has "settled."
		if err := updateAddr(ctx, pd); err != nil {
			log.Printf("failed to fetch endpoint: %v", err)
		}

		// Then, do it every 20 seconds.
		tick := time.NewTicker(time.Second * 20)
		for {
			select {
			case <-ctx.Done():
				tick.Stop()
				return
			case <-tick.C:
				if err := updateAddr(ctx, pd); err != nil {
					log.Printf("failed to fetch endpoint: %v", err)
				}
			}
		}
	}()

	return nil
}

func startup(ctx context.Context) error {
	if err := config.ReadConfigFile(); err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	dir := getRuntimeDir()
	unixSocket = filepath.Join(dir, fmt.Sprintf("pikonet.%d", os.Getpid()))

	if err := bindUnix(ctx); err != nil {
		return fmt.Errorf("failed to create UNIX socket: %w", err)
	}

	// Fetch a port if we need to
	if config.Cfg.ListenPort == 0 {
		config.Cfg.ListenPort = int(rand.Uint32()|(1<<10)) & 0xFFFF
	}

	var err error
	if eng, err = piko.NewEngine(piko.Config{
		Rendezvous: config.Cfg.Rendezvous,
		DeviceID:   config.Cfg.DeviceID,
		Token:      config.Cfg.Token,
		ListenPort: config.Cfg.ListenPort,
	}); err != nil {
		return fmt.Errorf("failed to start engine: %w", err)
	}

	if err := startWireguard(); err != nil {
		return fmt.Errorf("failed to start wireguard: %w", err)
	}

	go func() {
		log.Print(listenDNS())
	}()

	wgLock.Lock()
	wgDev.SetState(true)
	wgLock.Unlock()

	eng.OnJoin(wgOnJoin)
	eng.OnLeave(wgOnLeave)
	eng.OnUpdate(wgOnUpdate)
	eng.OnRebuild(wgOnRebuild)

	eng.OnJoin(dnsOnJoin)
	eng.OnLeave(dnsOnLeave)
	eng.OnUpdate(dnsOnUpdate)
	eng.OnRebuild(dnsOnRebuild)

	if err := eng.Connect(); err != nil {
		return fmt.Errorf("failed to connect to Rendezvous server: %w", err)
	}

	if err := startPikopunch(ctx); err != nil {
		return err
	}

	// Introduce ourselves now that we're all set up
	go listenBroadcast(ctx)

	return nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if os.Getuid() == 0 {
		log.Printf("Running with root privileges!")
	}

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)

	if err := startup(ctx); err != nil {
		log.Printf("Failed to start: %v", err)
		goto done
	}

	// Wait until we receive a SIGINT
	<-sigchan

done:
	log.Printf("Exiting.")

	if wgDev != nil {
		wgLock.Lock()
		wgDev.SetState(false)
		wgDev.Close()
		wgLock.Unlock()
	}

	cancel()
}
