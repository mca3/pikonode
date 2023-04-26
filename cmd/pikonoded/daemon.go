package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/internal/config"
)

// All of the waitGroup tomfoolery is because we have stuff to clean up on exit
// via defers and when main() exits, the defers on our couple of goroutines do
// not run.
// This is likely not an ideal solution.
var waitGroup = sync.WaitGroup{}

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
		return err
	}

	return rv.GatewaySend(ctx, api.GatewayMsg{
		Type:     api.Ping,
		DeviceID: ourDevice.ID,
		Endpoint: addr,
	})
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
	log.Printf("UNIX socket is at %v", unixSocket)

	// Fetch a port if we need to
	if config.Cfg.ListenPort == 0 {
		config.Cfg.ListenPort = int(rand.Uint32()|(1<<10)) & 0xFFFF
	}

	if err := createWireguard(ctx); err != nil {
		return fmt.Errorf("failed to create WireGuard interface: %w", err)
	}
	log.Printf("WireGuard interface is %v. Listen port is %d.", config.Cfg.InterfaceName, config.Cfg.ListenPort)

	go listenBroadcast(ctx)

	if err := createPikorv(ctx); err != nil {
		return fmt.Errorf("failed to connect to Rendezvous server: %w", err)
	}

	pd, err := rv.PunchDetails(ctx)
	if err != nil {
		return fmt.Errorf("failed to request pikopunch details: %w", err)
	}

	pdkey, err := parseKey(pd.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse pikopunch key: %w", err)
	}

	wgChan <- wgMsg{
		Type:     wgPeer,
		IP:       pd.IP,
		Endpoint: pd.Endpoint,
		Key:      pdkey,
	}

	// Introduce ourselves now that we're all set up
	sendDiscovHello(false)

	go func() {
		select {
		// Wait for WireGuard to settle
		case <-time.After(time.Second * 2):
		case <-ctx.Done():
			return
		}

		if err := updateAddr(ctx, pd); err != nil {
			log.Printf("failed to fetch endpoint: %v", err)
		}

		tick := time.NewTicker(time.Second * 20)

		for {
			select {
			case <-ctx.Done():
				tick.Stop()
			case <-tick.C:
				if err := updateAddr(ctx, pd); err != nil {
					log.Printf("failed to fetch endpoint: %v", err)
				}
			}
		}
	}()

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

	// Tell WireGuard about our IP
	wgChan <- wgMsg{
		Type: wgSetIP,
		IP:   ourDevice.IP,
	}

	// Up!
	wgChan <- wgMsg{
		Type: wgUp,
	}

	// Wait until we receive a SIGINT
	<-sigchan

done:
	log.Printf("Exiting.")

	wgChan <- wgMsg{
		Type: wgDown,
	}

	cancel()
	waitGroup.Wait()
}
