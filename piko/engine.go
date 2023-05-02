// Package piko ties together all code in this repository to provide an
// "Engine" interface, allowing control of Pikonet and how it works, alongside
// performing events when something happens.
package piko

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/mca3/pikonode/api"
)

// Engine is the main controller for Pikonet.
type Engine struct {
	cfg Config

	peerState []api.Device
	nwState   []api.Network

	ourDevice api.Device
	gw        chan api.GatewayMsg

	api *api.API

	onJoin    []func(nw *api.Network, dev *api.Device)
	onLeave   []func(nw *api.Network, dev *api.Device)
	onUpdate  []func(dev *api.Device)
	onRebuild []func()

	sync.Mutex
}

// Config holds configuration for the Pikonet engine.
type Config struct {
	// Token is the token that we authenticate to the Rendezvous server
	// with.
	Token string

	// Rendezvous is the URL of the rendezvous server.
	Rendezvous string

	// DeviceID is the ID of this device.
	DeviceID int64

	// ListenPort is the port that WireGuard communicates on.
	ListenPort int
}

// NewEngine creates a new instance of Engine.
func NewEngine(cfg Config) (*Engine, error) {
	e := &Engine{
		cfg: cfg,
		api: &api.API{
			Server: cfg.Rendezvous,
			Token:  cfg.Token,
			HTTP:   http.DefaultClient,
		},
		gw: make(chan api.GatewayMsg, 100),
	}

	return e, nil
}

// deviceIsIn returns true if a device ID is found within a network.
func deviceIsIn(needle api.Device, haystack []api.Device) bool {
	for _, v := range haystack {
		if needle.ID == v.ID {
			return true
		}
	}
	return false
}

// fetchNetwork returns a reference to a network in the cache.
func (e *Engine) fetchNetwork(id int64) *api.Network {
	for _, v := range e.nwState {
		if v.ID == id {
			return &v
		}
	}

	return nil
}

// rebuildState fully rebuilds the state objects.
func (e *Engine) rebuildState() {
	e.Lock()
	defer e.Unlock()

	e.nwState = e.nwState[:0]

	nws, err := e.api.Networks(context.TODO())
	if err != nil {
		return
		// return fmt.Errorf("failed to fetch networks: %v", err)
	}

	for _, v := range nws {
		ok := false
		for _, d := range v.Devices {
			if d.ID == e.ourDevice.ID {
				// That's us!
				ok = true
				break
			}
		}

		if ok {
			e.nwState = append(e.nwState, v)
		}
	}

	e.updatePeers()

	// Call all handlers.
	for _, v := range e.onRebuild {
		v()
	}
}

// updatePeers updates the peer list.
func (e *Engine) updatePeers() {
	// Assuming we're still locked.

	// Find old devices.
	valid := 0
	for _, d := range e.peerState {
		if d.ID == e.ourDevice.ID {
			// This should never be here.
			continue
		}

		ok := false
		for _, v := range e.nwState {
			if deviceIsIn(d, v.Devices) {
				ok = true
				break
			}
		}

		if !ok {
			continue
		}

		e.peerState[valid] = d
		valid++
	}

	e.peerState = e.peerState[:valid]

	// Find new devices
	for _, v := range e.nwState {
		for _, d := range v.Devices {
			if d.ID == e.ourDevice.ID {
				// This should never be here.
				continue
			}

			if !deviceIsIn(d, e.peerState) {
				e.peerState = append(e.peerState, d)
			}
		}
	}
}

// handleGateway waits for gateway messages.
func (e *Engine) handleGateway(ctx context.Context) {
	for {
		select {
		case v := <-e.gw:
			e.handleGwMsg(ctx, v)
		case <-ctx.Done():
			return
		}
	}
}

// handleGwMsg handles gateway messages.
func (e *Engine) handleGwMsg(ctx context.Context, msg api.GatewayMsg) {
	switch msg.Type {
	case api.NetworkJoin:
		e.handleJoin(msg.Network, msg.Device)
	case api.NetworkLeave:
		e.handleLeave(msg.Network, msg.Device)
	case api.DeviceUpdate:
		e.handleUpdate(msg.Device)
	case api.Connect:
		e.rebuildState()
	case api.Disconnect:
		// log.Printf("Disconnected from rendezvous server. Error: %v", msg.Error)
		// log.Printf("Reconnecting to rendezvous in %v", msg.Delay)
	}
}

// handleSelfJoin handles joining a network that our device has been joined to.
func (e *Engine) handleSelfJoin(nw *api.Network) {
	// We are assuming that are still locked here.

	_nw, err := e.api.Network(context.TODO(), nw.ID)
	if err != nil {
		// TODO: Complain
		return
	}

	e.nwState = append(e.nwState, _nw)
}

// handleJoin handles a device joining a network.
func (e *Engine) handleJoin(nw *api.Network, dev *api.Device) {
	e.Lock()
	defer e.Unlock()

	if nw == nil || dev == nil {
		// TODO: Complain.
		return
	}

	var cnw *api.Network

	if dev.ID == e.ourDevice.ID {
		// We need to handle our own changes mildly differently.
		e.handleSelfJoin(nw)
		goto done
	}

	// Handle state changes.
	cnw = e.fetchNetwork(nw.ID)
	if cnw == nil {
		// ???
		return
	}

	// TODO: Please stop doing this
	cnw.Devices = append(cnw.Devices, *dev)

done:
	e.updatePeers()

	// Call all handlers.
	for _, v := range e.onJoin {
		v(nw, dev)
	}
}

// handleSelfLeave handles leaving a network that our device has left.
func (e *Engine) handleSelfLeave(nw *api.Network) {
	// We are assuming that are still locked here.

	for i, v := range e.nwState {
		if v.ID != nw.ID {
			continue
		}

		e.nwState[i], e.nwState[len(e.nwState)-1] = e.nwState[len(e.nwState)-1], e.nwState[i]
		e.nwState = e.nwState[:len(e.nwState)-1]
		break
	}
}

// handleLeave handles a device leaving a network.
func (e *Engine) handleLeave(nw *api.Network, dev *api.Device) {
	e.Lock()
	defer e.Unlock()

	if nw == nil || dev == nil {
		// TODO: Complain.
		return
	}

	var cnw *api.Network

	if dev.ID == e.ourDevice.ID {
		// We need to handle our own changes mildly differently.
		e.handleSelfLeave(nw)
		goto done
	}

	// Handle state changes.
	cnw = e.fetchNetwork(nw.ID)
	if cnw == nil {
		// ???
		return
	}

	for i, v := range cnw.Devices {
		if v.ID != dev.ID {
			continue
		}

		cnw.Devices[i], cnw.Devices[len(cnw.Devices)-1] = cnw.Devices[len(cnw.Devices)-1], cnw.Devices[i]
		cnw.Devices = cnw.Devices[:len(cnw.Devices)-1]
	}

done:
	e.updatePeers()

	// Call all handlers.
	for _, v := range e.onLeave {
		v(nw, dev)
	}
}

// handleUpdate handles a device updating its details.
func (e *Engine) handleUpdate(dev *api.Device) {
	e.Lock()
	defer e.Unlock()

	if dev == nil {
		// TODO: Complain.
		return
	}

	if dev.ID == e.ourDevice.ID {
		e.ourDevice = *dev
	}

	// Update all networks
	for _, v := range e.nwState {
		for i, d := range v.Devices {
			if d.ID == dev.ID {
				v.Devices[i] = *dev
				break
			}
		}
	}

	e.updatePeers()

	// Call all handlers.
	for _, v := range e.onUpdate {
		v(dev)
	}
}

// Connect attempts to connect to the Rendezvous server.
func (e *Engine) Connect() error {
	// Try to get our device
	dev, err := e.api.Device(context.TODO(), e.cfg.DeviceID)
	if err != nil {
		return fmt.Errorf("failed to fetch our device: %w", err)
	}

	e.ourDevice = dev

	go e.api.Gateway(context.TODO(), e.gw, dev.ID, e.cfg.ListenPort)
	go e.handleGateway(context.TODO())

	return nil
}

// Peers returns a list of peers that this device has.
//
// Anything that is not a handler should lock the Engine object before
// performing reads.
func (e *Engine) Peers() []api.Device {
	return e.peerState
}

// Networks returns a list of networks that this device is connected to.
//
// Anything that is not a handler should lock the Engine object before
// performing reads.
func (e *Engine) Networks() []api.Network {
	return e.nwState
}

// Self returns the device that Engine is communicating to the Rendezvous
// server as.
//
// Anything that is not a handler should lock the Engine object before
// performing reads.
func (e *Engine) Self() *api.Device {
	return &e.ourDevice
}

// API returns the api.API object that is used by Engine.
func (e *Engine) API() *api.API {
	return e.api
}

// OnJoin registers a function to be called upon a device joining a network.
// The function will be called after the peer list has been updated.
//
// The function is guaranteed to have exclusive access on the Engine.
func (e *Engine) OnJoin(f func(nw *api.Network, dev *api.Device)) {
	e.onJoin = append(e.onJoin, f)
}

// OnLeave registers a function to be called upon a device leaving a network.
// The function will be called after the peer list has been updated.
//
// The function is guaranteed to have exclusive access on the Engine.
func (e *Engine) OnLeave(f func(nw *api.Network, dev *api.Device)) {
	e.onLeave = append(e.onLeave, f)
}

// OnUpdate registers a function to be called upon a device updating its
// information.
//
// The function is guaranteed to have exclusive access on the Engine.
func (e *Engine) OnUpdate(f func(dev *api.Device)) {
	e.onUpdate = append(e.onUpdate, f)
}

// OnRebuild registers a function to be called upon a full rebuild of the
// internal Pikonet state.
// The function will be called after the peer list has been updated.
//
// The function is guaranteed to have exclusive access on the Engine.
func (e *Engine) OnRebuild(f func()) {
	e.onRebuild = append(e.onRebuild, f)
}
