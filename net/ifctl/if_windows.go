package ifctl

import (
	"net"
	"net/netip"

	"golang.zx2c4.com/wireguard/windows/driver"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// winInterface implements support for Windows network adapters.
type winInterface struct {
	luid   winipcfg.LUID
	ad     *driver.Adapter
	routes []*winipcfg.RouteData
}

func asPrefix(ip *net.IPNet) netip.Prefix {
	// lol
	return netip.MustParsePrefix(ip.String())
}

// New creates a new WireGuard interface and returns an Interface object.
//
// By default, the created interface will be down and must be manually set up.
func New(name string) (Interface, error) {
	ad, err := driver.CreateAdapter(name, "Pikonet", nil) // TODO
	if err != nil {
		return nil, err
	}

	return &winInterface{
		ad:   ad,
		luid: ad.LUID(),
	}, nil
}

// From returns an Interface by its name.
func From(name string) (Interface, error) {
	ad, err := driver.OpenAdapter(name)
	if err != nil {
		return nil, err
	}

	return &winInterface{
		ad:   ad,
		luid: ad.LUID(),
	}, nil
}

// Set sets the state of the interface to be up or down.
func (wi *winInterface) Set(state bool) error {
	if state {
		return wi.ad.SetAdapterState(driver.AdapterStateUp)
	}
	return wi.ad.SetAdapterState(driver.AdapterStateDown)
}

// SetAddr sets the address of the interface to the one provided.
func (wi *winInterface) SetAddr(addr *net.IPNet) error {
	pfx := asPrefix(addr)
	return wi.luid.SetIPAddresses([]netip.Prefix{pfx})
}

// Add adds a route for an IPNet to the interface.
func (wi *winInterface) AddRoute(route *net.IPNet) error {
	pfx := asPrefix(route)

	// We won't add this route if we already have one configured for this address.
	for _, v := range wi.routes {
		if v.Destination.Contains(pfx.Addr()) {
			return nil
		}
	}

	rd := &winipcfg.RouteData{
		Destination: pfx,
		NextHop:     netip.IPv6Unspecified(),
	}
	wi.routes = append(wi.routes, rd)

	return wi.luid.SetRoutes(wi.routes)
}

// DeleteRoute removes a route for an IPNet to the interface.
func (wi *winInterface) DeleteRoute(route *net.IPNet) error {
	pfx := asPrefix(route)

	for i, v := range wi.routes {
		if v.Destination.Contains(pfx.Addr()) {
			wi.routes[i], wi.routes[len(wi.routes)-1] = wi.routes[len(wi.routes)-1], wi.routes[i]
			wi.routes = wi.routes[:len(wi.routes)-1]
		}
	}

	return wi.luid.SetRoutes(wi.routes)
}

// Delete deletes the interface.
func (wi *winInterface) Delete() error {
	return wi.ad.Close()
}
