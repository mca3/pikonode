//go:build !windows

package wg

// This exists because uapiOpen does not exist on Windows, and I don't want to
// copy code more than I need to.

import (
	"net"
	"os"

	"golang.zx2c4.com/wireguard/ipc"
)

func uapiOpen(name string) (*os.File, error) {
	return ipc.UAPIOpen(name)
}

func uapiListen(name string, file *os.File) (net.Listener, error) {
	return ipc.UAPIListen(name, file)
}
