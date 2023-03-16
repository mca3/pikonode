package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

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

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if os.Getuid() == 0 {
		log.Printf("Running with root privileges!")
	}

	if err := config.ReadConfigFile(); err != nil {
		log.Fatalf("Failed to load config file: %v", err)
	}

	dir := getRuntimeDir()
	unixSocket = filepath.Join(dir, fmt.Sprintf("pikonet.%d", os.Getpid()))

	if err := bindUnix(ctx); err != nil {
		log.Fatalf("Failed to create UNIX socket: %v", err)
	}
	log.Printf("UNIX socket is at %v", unixSocket)

	if err := createWireguard(ctx); err != nil {
		log.Fatalf("Failed to create WireGuard interface: %v", err)
	}
	log.Printf("WireGuard interface is %v. Listen port is %d.", config.Cfg.InterfaceName, config.Cfg.ListenPort)

	// Wait until we receive a SIGINT
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	<-sigchan

	log.Printf("Exiting.")

	cancel()
	waitGroup.Wait()
}
