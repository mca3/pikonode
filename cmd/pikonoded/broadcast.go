package main

// This file deals with UDP broadcasting to automatically discover peers and
// connect to them.
// If you see references to the "Discovery" protocol, it's this file.
//
// Eventually, this should be rewritten to use multicast instead since it's the
// better way to do this kind of thing, or so I'm told.
//
// ---
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

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"
	"net"
	"time"

	"github.com/mca3/pikonode/cmd/pikonoded/wg"
	"github.com/mca3/pikonode/internal/config"
)

const (
	discovSize = 512
	discovPort = 0x706b // 28779
)

// Discovery message commands.
const (
	// 0x00 - Reserved for future revisions.
	_ byte = 0x00

	// 0x01 - Hello
	// Broadcasts your existance on the network.
	//
	// Payload:
	// - uint16: Listening port for WireGuard
	// - [44]byte: Base64 WireGuard public key.
	discovTypeHello = 0x01

	// 0x02 - Hello Reply
	// Broadcasts your existance on the network.
	//
	// The payload of this is the same as 0x01 Hello; this type explicitly
	// marks this message as a reply to an earlier Hello message.
	// Send these as a reply to 0x01, and not in any other case.
	//
	// Payload:
	// - uint16: Listening port for WireGuard
	// - [44]byte: Base64 WireGuard public key.
	discovTypeHelloReply = 0x02
)

// UDP Broadcast socket.
//
// Used for asking the local network about devices on the network.
var udpBrd *net.UDPConn

var discovHelloTicker = time.NewTicker(time.Minute)

// listenBroadcast listens for discovery packets on the local interface.
func listenBroadcast(ctx context.Context) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: discovPort,
	})
	if err != nil {
		return err
	}

	udpBrd = conn

	go discovHello(ctx)

	go func() {
		// Close when context is finished.

		<-ctx.Done()
		conn.Close()
	}()

	go func() {
		log.Fatal(discovWait())
	}()

	return nil
}

// discovWait reads discovery messages from the broadcast IP.
func discovWait() error {
	buf := make([]byte, discovSize)

	for {
		n, addr, err := udpBrd.ReadFrom(buf)
		if err != nil {
			return err
		}

		if n <= 5 {
			// Too small
			continue
		}

		msg := buf[:n]

		// Determine header
		if !bytes.Equal(msg[:4], []byte("PIKO")) {
			continue
		}

		switch msg[4] {
		case discovTypeHello, discovTypeHelloReply:
			// These packets have the same payload and same
			// meaning, just you can't reply to one of them.
			if len(msg) >= 5+2+44 {
				onDiscovHello(addr, msg[5:], msg[4] == discovTypeHelloReply)
			}
		default:
			log.Printf("Unknown discovery message type %02x from %s", msg[4], addr)
		}
	}
}

// sendDiscovHello sends a Hello message to the network.
//
// If reply is true, a Hello Reply message will be sent.
func sendDiscovHello(reply bool) {
	// Don't send another automatic Hello for at least another minute
	discovHelloTicker.Reset(time.Minute)

	// Layout: "PIKO" HELLO (port) (public key)
	buf := make([]byte, 51)

	copy(buf[:4], "PIKO")

	if reply {
		// The semantics of Hello Reply is the same, but Hello Reply
		// must not be sent as a result of another Hello Reply.
		// Only as a result of a Hello.
		buf[4] = discovTypeHelloReply
	} else {
		buf[4] = discovTypeHello
	}

	binary.BigEndian.PutUint16(buf[5:7], uint16(config.Cfg.ListenPort))
	copy(buf[7:], config.Cfg.PublicKey)

	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4bcast, Port: discovPort})
	if err != nil {
		return
	}
	defer conn.Close()

	conn.Write(buf)
}

// discovHello automatically sends out a Hello message every minute.
func discovHello(ctx context.Context) {
	for {
		select {
		case <-discovHelloTicker.C:
			sendDiscovHello(false)
		case <-ctx.Done():
			return
		}
	}
}

// onDiscovHello performs actions based on a received Hello message.
//
// If the Hello message is not a Hello Reply, a Hello Reply will be sent.
func onDiscovHello(addr net.Addr, msg []byte, reply bool) {
	port := binary.BigEndian.Uint16(msg[:2])
	key := string(msg[2:46])

	if key == config.Cfg.PublicKey {
		// Ignore ourselves
		return
	}

	log.Printf("HELLO from %s, port %d public key %s", addr, port, key)

	if !reply {
		// We may only send a reply when the message wasn't a Hello
		// Reply; this is to prevent flooding the network.
		// In this case, it isn't.
		sendDiscovHello(true)
	}

	// Determine if we want to connect to them
	// TODO: Determine if an existing connection is good and ignore this?
	// TODO: This is racy.
	ok := false
	for _, v := range peerList {
		if v.PublicKey == key {
			ok = true
			break
		}
	}

	if !ok {
		return
	}

	// We want to connect to them.
	// Bypassing whatever Rendezvous thinks.
	pkey, err := wg.ParseKey(key)
	if err != nil {
		log.Printf("HELLO from %s sent invalid public key!", addr)
		return
	}

	ep := addr.(*net.UDPAddr)
	ep.Port = int(port)

	wgLock.Lock()
	wgDev.AddPeer(nil, ep, pkey)
	wgLock.Unlock()
}
