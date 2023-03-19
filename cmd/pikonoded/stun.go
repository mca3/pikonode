package main

import (
	"log"
	"net"

	"github.com/pion/stun"
)

const stunAddr = "stunserver.stunprotocol.org:3478"

// fetchPort returns the local and external port for this machine.
//
// The local port shall be WireGuard's listen port.
// The external port shall be reported to the rendezvous server.
func fetchPort() (int, int, error) {
	conn, err := net.Dial("udp4", stunAddr)
	if err != nil {
		return -1, -1, err
	}
	defer conn.Close()

	log.Println(conn.LocalAddr())

	c, err := stun.NewClient(conn)
	if err != nil {
		return -1, -1, err
	}
	defer c.Close()

	port := make(chan int, 1)

	err = c.Do(stun.MustBuild(stun.TransactionID, stun.BindingRequest), func(res stun.Event) {
		if res.Error != nil {
			log.Fatalln(res.Error)
		}
		var xorAddr stun.XORMappedAddress
		if getErr := xorAddr.GetFrom(res.Message); getErr != nil {
			log.Fatalln(getErr)
		}
		port <- xorAddr.Port
	})

	return conn.LocalAddr().(*net.UDPAddr).Port, <-port, err
}
