package ifctl

import (
	"net"

	"github.com/vishvananda/netlink"
)

// linuxInterface implements support for Linux interfaces using the native
// Netlink interface.
type linuxInterface struct {
	link   netlink.Link
	routes []netlink.Route

	// linkAttrs is needed because we need to implement netlink.Link very
	// temporarily.
	linkAttrs netlink.LinkAttrs
}

// For netlink.Link. Do not use.
func (li *linuxInterface) Attrs() *netlink.LinkAttrs {
	return &li.linkAttrs
}

// For netlink.Link. Do not use.
func (li *linuxInterface) Type() string {
	return "wireguard"
}

// New creates a new WireGuard interface and returns an Interface object.
//
// By default, the created interface will be down and must be manually set up.
func New(name string) (Interface, error) {
	li := linuxInterface{}
	li.linkAttrs = netlink.NewLinkAttrs()
	li.linkAttrs.Name = name

	err := netlink.LinkAdd(&li)
	if err != nil {
		return nil, err
	}

	l, err := netlink.LinkByName(li.linkAttrs.Name)
	if err != nil {
		return nil, err
	}

	li.link = l
	return &li, nil
}

// From returns an Interface by its name.
func From(name string) (Interface, error) {
	li := linuxInterface{}

	l, err := netlink.LinkByName(name)
	if err != nil {
		return nil, err
	}

	li.link = l
	return &li, nil
}

// Name returns the name of the interface.
func (li *linuxInterface) Name() string {
	return li.linkAttrs.Name
}

// Set sets the state of the interface to be up or down.
func (li *linuxInterface) Set(state bool) error {
	if state {
		if err := netlink.LinkSetUp(li.link); err != nil {
			return err
		}

		// Add all routes back. Quietly fail.
		// TODO: No-op if already up.
		for _, v := range li.routes {
			netlink.RouteAdd(&v)
		}

		return nil
	}

	return netlink.LinkSetDown(li.link)
}

// SetAddr sets the address of the interface to the one provided.
func (li *linuxInterface) SetAddr(addr *net.IPNet) error {
	// TODO: Remove old address?
	return netlink.AddrAdd(li.link, &netlink.Addr{IPNet: addr})
}

// Add adds a route for an IPNet to the interface.
func (li *linuxInterface) AddRoute(route *net.IPNet) error {
	// We won't add this route if we already have one configured for this address.
	for _, v := range li.routes {
		if v.Dst.Contains(route.IP) {
			return nil
		}
	}

	r := netlink.Route{
		LinkIndex: li.link.Attrs().Index,
		Protocol:  6,
		Dst:       route,
	}
	li.routes = append(li.routes, r)

	// TODO: Determine if link is up or not so it does not error.
	return netlink.RouteAdd(&r)
}

// DeleteRoute removes a route for an IPNet to the interface.
func (li *linuxInterface) DeleteRoute(route *net.IPNet) error {
	for i, v := range li.routes {
		if v.LinkIndex == li.link.Attrs().Index && v.Dst.Contains(route.IP) {
			li.routes[i], li.routes[len(li.routes)-1] = li.routes[len(li.routes)-1], li.routes[i]
			li.routes = li.routes[:len(li.routes)-1]

			// TODO: Determine if link is up or not so it does not error.
			return netlink.RouteDel(&v)
		}
	}

	return nil
}

// Delete deletes the interface.
func (li *linuxInterface) Delete() error {
	return netlink.LinkDel(li.link)
}
