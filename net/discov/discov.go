// This package deals with UDP broadcasting to automatically discover peers and
// connect to them.
// If you see references to the "Discovery" protocol, it's this package.
//
// # Background
//
// The Pikonet discovery protocol is a simple protocol operating on port 28779.
//
// All messages have a 4 byte "PIKO" header, a one byte command type, and then
// a payload of arbitrary length.
// Messages should be sent to the appropriate broadcast address.
//
// The protocol was created to allow efficient automatic configuration on
// private networks without telling the outside world.
// It allows communication across the same network as without it, it would be
// impossible to communicate with another device on the same network
// ordinarily.
//
// It was not created for security, and it could lead to denial of service;
// these problems *may* be solved in the future but not necessarily will be
// solved.
package discov

import (
	"bytes"
	"context"
	"net"
	"sync"

	"golang.org/x/net/ipv4"
)

const (
	// Maximum discovery message size.
	discovSize = 512
)

const (
	// The port that Discovery communicates on.
	Port = 28779
)

var (
	// Multicast IP for Discovery communication.
	IP = net.IPv4(239, 112, 110, 100)

	// IP and Port combined into a UDPAddr.
	Address = &net.UDPAddr{
		IP:   IP,
		Port: Port,
	}
)

// Discovery holds state for local peer discovery.
type Discovery struct {
	ifs []net.Interface
	pc  *ipv4.PacketConn
	mu  sync.Mutex

	// Ready is a channel that is closed when messages are ready to be sent
	// or received.
	Ready chan struct{}

	// Notify is a function that is called upon receiving a Discovery
	// message.
	Notify func(addr *net.UDPAddr, m Message)
}

// Listen listens for discovery messages on the local network.
//
// The error returned will always be non-nil.
func (d *Discovery) Listen(ctx context.Context) error {
	ifs, err := fetchInterfaces()
	if err != nil {
		return err
	}

	// TODO: Best effort for without interfaces: 255.255.255.255?
	// Or even for a failure trying to setup multicast, for that matter.

	c, err := net.ListenPacket("udp4", Address.String())
	if err != nil {
		return err
	}
	defer c.Close()

	pc := ipv4.NewPacketConn(c)

	d.mu.Lock()
	d.ifs = ifs
	d.pc = pc
	d.mu.Unlock()

	// Join the multicast group on all interfaces.
	for _, v := range ifs {
		v := v

		pc.JoinGroup(&v, Address)
		defer pc.LeaveGroup(&v, Address)
	}

	// Notify everyone we're ready
	if d.Ready != nil {
		close(d.Ready)
	}

	// Listen for packets.
	buf := make([]byte, discovSize)

	for {
		bytesRead, _, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return err
		}

		if bytesRead < 5 {
			// Minimum message length is 5 bytes
			continue
		}

		msg := buf[:bytesRead]

		if !bytes.Equal(msg[:4], []byte("PIKO")) {
			// Invalid header
			continue
		}

		if d.Notify != nil {
			d.Notify(addr.(*net.UDPAddr), Parse(msg[:]))
		}
	}
}

// Send sends a message to the multicast group.
func (d *Discovery) Send(msg []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, v := range d.ifs {
		if err := d.pc.SetMulticastInterface(&v); err == nil {
			d.pc.WriteTo(msg, nil, Address)
		}
	}
}

// isGoodInterface determines if this interface is suitable for multicast.
func isGoodInterface(ifc net.Interface) bool {
	// Things the interface must be:
	// - Up
	// - Not loopback
	// - Supports multicast

	// TODO: It should also support the address family we're looking for,
	// because I only have set this up assuming most will still be dealing
	// with IPv4 on a daily basis. Things change however!

	if ifc.Flags&net.FlagUp == 0 {
		return false
	} else if ifc.Flags&net.FlagLoopback != 0 {
		return false
	} else if ifc.Flags&net.FlagMulticast == 0 {
		return false
	}

	return true
}

// fetchInterfaces determines a suitable set of interfaces for multicast.
func fetchInterfaces() ([]net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	valid := 0
	for _, v := range ifaces {
		if isGoodInterface(v) {
			// In place filtering
			ifaces[valid] = v
			valid++
		}
	}

	return ifaces[:valid], nil
}
