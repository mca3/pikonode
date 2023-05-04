package main

import (
	"net"
	"strings"
	"sync"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/net/dns"
)

type dnsRecord struct {
	IP net.IP
}

// dnsMap is a mapping between DNS keys and an IP.
// dnsMap is protected by dnsMut.
var dnsMap = map[string]dnsRecord{}
var dnsMut = sync.RWMutex{}

// lookupDns looks up a DNS key.
func lookupDns(key string) (dnsRecord, bool) {
	dnsMut.RLock()
	defer dnsMut.RUnlock()

	v, ok := dnsMap[key]
	return v, ok
}

// listenDNS listens for DNS queries.
func listenDNS() error {
	srv := dns.Server{
		Fallback: "1.1.1.1:53",
		Resolve: func(q []string) (net.IP, bool) {
			if len(q) == 0 {
				return net.IP(nil), false
			}

			val, ok := lookupDns(q[len(q)-1])
			return val.IP, ok
		},
		Suffix: []string{"pn", "local"},
	}

	return srv.Listen(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53})
}

// domainify converts name into something which may be included in a domain name.
func domainify(name string) string {
	// TODO: Do this properly; punycode?
	mapper := func(r rune) rune {
		// matches [0-9a-z]; all that don't match become '-'
		if (r >= 48 && r <= 57) || (r >= 97 && r <= 122) {
			return r
		}
		return '-'
	}

	return strings.Map(mapper, strings.ToLower(name))
}

func dnsOnJoin(nw *api.Network, dev *api.Device) {
	dnsUpdatePeers()
}

func dnsOnLeave(nw *api.Network, dev *api.Device) {
	dnsUpdatePeers()
}

func dnsOnUpdate(dev *api.Device) {
	// TODO: Name changes are not yet supported
	dnsUpdatePeers()
}

func dnsOnRebuild() {
	dnsUpdatePeers()
}

func dnsUpdatePeers() {
	dnsMut.Lock()
	defer dnsMut.Unlock()

	peers := eng.Peers()
	for _, v := range peers {
		dnsMap[domainify(v.Name)] = dnsRecord{IP: net.ParseIP(v.IP)}
	}

	// Add ourselves
	// No need to lock because we have exclusive access
	dnsMap[domainify(eng.Self().Name)] = dnsRecord{IP: net.ParseIP(eng.Self().IP)}
}
