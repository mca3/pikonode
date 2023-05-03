package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/mca3/pikonode/internal/config"
	"github.com/mca3/pikonode/net/discov"
	"github.com/mca3/pikonode/net/wg"
)

type discovPeer struct {
	LastSeen time.Time
	Endpoint string
}

const (
	// Controls the amount of time since a HELLO we will consider a local
	// peer as alive.
	//
	// When time.Now().Add(discovGracePeriod).Before(peer.LastSeen), the
	// discovPeer is valid.
	discovGracePeriod = time.Minute * 2
)

var discovConn *discov.Discovery

var discovHelloTicker = time.NewTicker(time.Minute)

// seenPeers holds all local peers that have sent a HELLO.
// seenPeers is protected by seenMut.
//
// TODO: Should we GC occasionally?
var seenPeers = map[string]discovPeer{}
var seenMut sync.Mutex

// Valid returns true if the peer has sent a HELLO recently.
func (d discovPeer) Valid() bool {
	return time.Now().Add(discovGracePeriod).Before(d.LastSeen)
}

// localPeer looks up the specified public key to determine if a peer has sent
// a HELLO on the local network, and if it has done so recently, will return
// the peer's information and true.
func localPeer(key string) (discovPeer, bool) {
	seenMut.Lock()
	defer seenMut.Unlock()

	for k, v := range seenPeers {
		if k == key && v.Valid() {
			return v, true
		}
	}

	return discovPeer{}, false
}

// listenBroadcast listens for discovery packets on the local interface.
func listenBroadcast(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	discovConn = &discov.Discovery{
		Notify: onDiscovMessage,
		Ready:  make(chan struct{}),
	}

	errCh := make(chan error)
	go func() {
		errCh <- discovConn.Listen(ctx)
	}()

	// Wait to send the first Hello by waiting until we're ready.
	<-discovConn.Ready
	sendDiscovHello(false)

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case <-discovHelloTicker.C:
			sendDiscovHello(false)
		}
	}
}

// sendDiscovHello sends a Hello message to the network.
//
// If reply is true, a Hello Reply message will be sent.
func sendDiscovHello(reply bool) {
	// Don't send another automatic Hello for at least another minute
	discovHelloTicker.Reset(time.Minute)
	discovConn.Send(discov.NewHello(uint16(config.Cfg.ListenPort), config.Cfg.PublicKey, reply))
}

// onDiscovHello performs actions based on a received Hello message.
//
// If the Hello message is not a Hello Reply, a Hello Reply will be sent.
func onDiscovMessage(addr *net.UDPAddr, msg discov.Message) {
	if msg.Type != discov.Hello && msg.Type != discov.HelloReply {
		return
	} else if msg.Key == config.Cfg.PublicKey {
		// Ignore ourselves
		return
	}

	log.Printf("HELLO from %s, port %d public key %s", addr, msg.Port, msg.Key)

	if msg.Type != discov.HelloReply {
		// We may only send a reply when the message wasn't a Hello
		// Reply; this is to prevent flooding the network.
		// In this case, it isn't.
		sendDiscovHello(true)
	}

	// The address of the WireGuard connection is the address of the
	// message that we have received this from, except with the port set to
	// the one specified in the message.
	addr.Port = int(msg.Port)

	// Add them to the cache
	seenMut.Lock()
	seenPeers[msg.Key] = discovPeer{LastSeen: time.Now(), Endpoint: addr.String()}
	seenMut.Unlock()

	// Determine if we want to connect to them
	// TODO: Determine if an existing connection is good and ignore this?
	eng.Lock()
	defer eng.Unlock()

	ok := false
	for _, v := range eng.Peers() {
		if v.PublicKey == msg.Key {
			ok = true
			break
		}
	}

	if !ok {
		return
	}

	// We want to connect to them.
	// Bypassing whatever Rendezvous thinks.
	pkey, err := wg.ParseKey(msg.Key)
	if err != nil {
		log.Printf("HELLO from %s sent invalid public key!", addr)
		return
	}

	// Add the new peer.
	// Note that often during startup this will get overridden, so this
	// isn't the only place where peers are set when discovered locally.
	wgLock.Lock()
	wgDev.AddPeer(nil, addr, pkey)
	wgLock.Unlock()
}
