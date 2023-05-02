// Package ifctl declares cross-platform helpers for creating and managing
// network interfaces.
package ifctl

import (
	"net"
)

type Interface interface {
	// Set sets the state of the interface to be up or down.
	Set(state bool) error

	// SetAddr sets the address of the interface to the one provided.
	SetAddr(addr *net.IPNet) error

	// Add adds a route for an IPNet to the interface.
	AddRoute(route *net.IPNet) error

	// DeleteRoute removes a route for an IPNet to the interface.
	DeleteRoute(route *net.IPNet) error

	// Delete deletes the interface.
	Delete() error
}
