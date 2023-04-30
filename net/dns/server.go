// Package dns implements a basic DNS server that can resolve addresses for
// Pikonet, or otherwise forward queries onto another DNS server.
package dns

import (
	"bytes"
	"fmt"
	"net"
	"sync"
)

// Server implements a basic DNS server that attempts to resolve for Pikonet
// first, but then falls back on other DNS servers.
type Server struct {
	// Fallback holds a fallback DNS server.
	//
	// Fallback is used when a Pikonet address returns no results.
	// If empty, then queries that reach this point return NXDOMAIN.
	Fallback string

	// Resolve is the function called when a DNS query is received.
	// If nil, then the server essentially acts as a proxy to the fallback
	// DNS servers.
	Resolve func(query []string) (result net.IP, ok bool)

	// Suffix holds the domain suffix (such as "com" for a domain that is
	// or ends with ".com").
	// When a query that matches this suffix is received, then the query
	// calls Resolve.
	//
	// If empty, then the server essentially acts as a proxy to the
	// fallback DNS servers.
	Suffix []string
}

var (
	bbufPool = sync.Pool{
		New: func() any {
			return &bytes.Buffer{}
		},
	}
)

// canResolve returns true if the value of our suffix is found at the end of
// labels.
func (s *Server) canResolve(labels []string) bool {
	if len(labels) < len(s.Suffix) {
		return false
	}

	labels = labels[len(labels)-len(s.Suffix):]

	for i := 0; i < len(s.Suffix); i++ {
		if labels[i] != s.Suffix[i] {
			return false
		}
	}

	return true
}

// fail sends an error message to the client.
func (s *Server) fail(uc *net.UDPConn, addr *net.UDPAddr, msg dnsMessage, code dnsRespCode) error {
	buf := bbufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bbufPool.Put(buf)

	retMsg := dnsMessage{
		ID:        msg.ID,
		QR:        false,
		RA:        true,
		Opcode:    opQuery,
		Resp:      code,
		Questions: msg.Questions,
	}

	retMsg.serialize(buf)

	_, err := uc.WriteTo(buf.Bytes(), addr)
	return err
}

// fallbackResolve attempts to resolve the DNS query using the fallback resolvers.
// Otherwise, it returns NXDOMAIN.
func (s *Server) fallbackResolve(uc *net.UDPConn, addr *net.UDPAddr, msg dnsMessage) error {
	if s.Fallback == "" {
		return s.fail(uc, addr, msg, respNXDomain)
	}

	// Reserialize the query.
	buf := bbufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bbufPool.Put(buf)

	msg.serialize(buf)

	c, err := net.Dial("udp", s.Fallback)
	if err != nil {
		return err
	}
	defer c.Close()

	// Write out our message to the other end.
	c.Write(buf.Bytes())

	// TODO: Should this be a pool?
	respBuf := make([]byte, 64*1024)
	n, err := c.Read(respBuf)
	if err != nil {
		return err
	}

	// We need to parse the DNS message and notify someone
	// if it's a valid response.
	msg, err = parseDNSMessage(respBuf[:n])
	if err != nil {
		return err
	}

	buf.Reset()
	msg.RA = true
	msg.serialize(buf)
	_, err = uc.WriteTo(buf.Bytes(), addr)
	return err
}

// handleQuery handles a DNS query.
func (s *Server) handleQuery(uc *net.UDPConn, addr *net.UDPAddr, msg dnsMessage) error {
	if len(msg.Questions) != 1 {
		// Literally nobody supports having more than 1 question in a
		// query, despite the packet format supporting it.
		// The semantics behind how return codes should be handled
		// aren't (well) defined.
		//
		// Also, we will fail zero queries.
		return s.fail(uc, addr, msg, respFormatErr)
	}

	// Determine if we can't handle this query.
	if msg.Questions[0].Type != typeAAAA && s.canResolve(msg.Questions[0].Labels) {
		// Domain found but no A record.
		return s.fail(uc, addr, msg, respOk)
	} else if !s.canResolve(msg.Questions[0].Labels) || s.Resolve == nil {
		return s.fallbackResolve(uc, addr, msg)
	}

	// We can handle this query.
	buf := bbufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bbufPool.Put(buf)

	result, ok := s.Resolve(msg.Questions[0].Labels[:len(msg.Questions[0].Labels)-len(s.Suffix)])
	if !ok {
		return s.fail(uc, addr, msg, respNXDomain)
	}

	msg.Answers = append(msg.Answers, dnsRecord{
		Labels: msg.Questions[0].Labels,
		Type:   msg.Questions[0].Type,
		Class:  msg.Questions[0].Class,
		TTL:    600, // TODO
		RData:  []byte(result),
	})
	msg.QR = false
	msg.RA = true

	msg.serialize(buf)
	_, err := uc.WriteTo(buf.Bytes(), addr)
	return err
}

func (s *Server) Listen(addr *net.UDPAddr) error {
	uc, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	defer uc.Close()

	fmt.Println(uc.LocalAddr())

	// TODO: Can this be done better?
	buf := make([]byte, 64*1024)
	for {
		n, addr, err := uc.ReadFrom(buf)
		if err != nil {
			return err
		}

		nbuf := bbufPool.Get().(*bytes.Buffer)
		nbuf.Reset()
		nbuf.Write(buf[:n])

		go func() {
			defer bbufPool.Put(nbuf)

			msg, err := parseDNSMessage(nbuf.Bytes())
			if err != nil {
				s.fail(uc, addr.(*net.UDPAddr), msg, respFormatErr)
				return
			}

			s.handleQuery(uc, addr.(*net.UDPAddr), msg)
		}()
	}
}
