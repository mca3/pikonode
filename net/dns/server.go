// Package dns implements a basic DNS server that can resolve addresses for
// Pikonet, or otherwise forward queries onto another DNS server.
package dns

import (
	"bytes"
	"fmt"
	"net"
)

var (
	ourSuffix = [][]byte{
		[]byte("pn"),
		[]byte("local"),
	}
)

// canResolve returns true if the value of ourSuffix is found at the end of
// labels.
func canResolve(labels [][]byte) bool {
	if len(labels) < len(ourSuffix) {
		return false
	}

	labels = labels[len(labels)-len(ourSuffix):]

	for i := 0; i < len(ourSuffix); i++ {
		if !bytes.Equal(labels[i], ourSuffix[i]) {
			return false
		}
	}

	return true
}

func sendServerFail(uc *net.UDPConn, addr *net.UDPAddr, msg dnsMessage, code dnsRespCode) {
	buf := &bytes.Buffer{}

	retMsg := dnsMessage{
		ID:        msg.ID,
		QR:        false,
		Opcode:    opQuery,
		Resp:      code,
		Questions: msg.Questions,
	}

	retMsg.serialize(buf)
	uc.WriteTo(buf.Bytes(), addr)
}

func handleMessage(uc *net.UDPConn, addr *net.UDPAddr, msg dnsMessage) {
	obuf := &bytes.Buffer{}

	if len(msg.Questions) != 1 {
		// Literally nobody supports having more than 1 question in a
		// query, despite the packet format supporting it.
		// The semantics behind how return codes should be handled
		// aren't (well) defined.
		sendServerFail(uc, addr, msg, respFormatErr)
		return
	}

	if canResolve(msg.Questions[0].Labels) {
		sendServerFail(uc, addr, msg, respNXDomain)
		return
	}

	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 53})
	if err != nil {
		sendServerFail(uc, addr, msg, respServerFail)
		return
	}
	defer c.Close()

	msg.serialize(obuf)
	c.Write(obuf.Bytes())
	obuf.Reset()

	buf := make([]byte, 64*1024)
	n, err := c.Read(buf)
	if err != nil {
		sendServerFail(uc, addr, msg, respServerFail)
		return
	}

	buf = buf[:n]

	nmsg, err := parseDNSMessage(buf)
	if err != nil {
		sendServerFail(uc, addr, msg, respFormatErr)
		return
	}

	nmsg.serialize(obuf)
	uc.WriteTo(obuf.Bytes(), addr)
}

func Listen() error {
	uc, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return err
	}
	defer uc.Close()

	fmt.Println(uc.LocalAddr())

	buf := make([]byte, 64*1024)
	for {
		n, addr, err := uc.ReadFrom(buf)
		if err != nil {
			return err
		}

		buf := buf[:n]
		msg, err := parseDNSMessage(buf)
		if err != nil {
			sendServerFail(uc, addr.(*net.UDPAddr), msg, respFormatErr)
			continue
		}

		handleMessage(uc, addr.(*net.UDPAddr), msg)
	}
}
