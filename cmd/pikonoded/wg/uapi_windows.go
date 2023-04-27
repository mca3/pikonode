package wg

// This exists because uapiOpen does not exist on Windows, and I don't want to
// copy code more than I need to.

import (
	"io"
	"net"

	"golang.zx2c4.com/wireguard/ipc"
)

func uapiOpen(name string) (io.Closer, error) {
	// I just need something that can close. It doesn't have to work.
	return io.NopCloser(nil), nil
}

func uapiListen(name string, file io.Closer) (net.Listener, error) {
	return ipc.UAPIListen(name)
}
