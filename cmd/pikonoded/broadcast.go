package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/mca3/pikonode/internal/config"
	"github.com/mca3/pikonode/net/discov"
	"github.com/mca3/pikonode/net/wg"
)

var discovConn *discov.Discovery

var discovHelloTicker = time.NewTicker(time.Minute)

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

	addr.Port = int(msg.Port)

	wgLock.Lock()
	wgDev.AddPeer(nil, addr, pkey)
	wgLock.Unlock()
}
