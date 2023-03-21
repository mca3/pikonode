package api

import (
	"context"
	"math"
	"net/http"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type gatewayMsgType int

type GatewayMsg struct {
	Type gatewayMsgType

	Device  *Device  `json:"device,omitempty"`
	Network *Network `json:"network,omitempty"`
	Remove  bool

	Endpoint  string `json:"endpoint,omitempty"`
	DeviceID  int64  `json:"device_id,omitempty"`
	NetworkID int64  `json:"network_id,omitempty"`

	Delay time.Duration `json:"-"`
	Error error         `json:"-"`
}

const (
	Ping gatewayMsgType = iota
	NetworkJoin
	NetworkLeave
	DeviceUpdate

	Disconnect gatewayMsgType = -1
	Connect    gatewayMsgType = -2
)

func (a *API) dialGateway(ctx context.Context) (*websocket.Conn, error) {
	c, _, err := websocket.Dial(ctx, a.Endpoint(EndpointGateway), &websocket.DialOptions{
		HTTPClient: a.HTTP,
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + a.Token},
		},
	})
	return c, err
}

func (a *API) gwReadLoop(ctx context.Context, conn *websocket.Conn, c chan<- GatewayMsg) error {
	for {
		msg := GatewayMsg{}
		err := wsjson.Read(ctx, conn, &msg)
		if err != nil {
			return err
		}

		// TODO: Some messages may control this

		select {
		case c <- msg:
		case <-ctx.Done():
			return nil
		}
	}
}

func (a *API) gwConnected(ctx context.Context, conn *websocket.Conn, dev int64, port int) {
	_ = wsjson.Write(ctx, conn, GatewayMsg{
		Type:     Ping,
		DeviceID: dev,
	})
}

func (a *API) Gateway(ctx context.Context, c chan<- GatewayMsg, dev int64, port int) {
	t := time.NewTimer(0)
	d := time.Second

	defer func() {
		if !t.Stop() {
			<-t.C
		}
	}()

	a.wsLock.Lock()

	for {
		conn, err := a.dialGateway(ctx)
		if err != nil {
			// Exponental backoff, max 10 mins
			d = time.Duration(math.Min(float64(d<<1), float64(time.Minute*10)))
			t.Reset(d)

			// Tell channel we're disconnected, and mention our delay
			c <- GatewayMsg{Type: Disconnect, Delay: d, Error: err}

			goto wait
		}

		// Successful connection
		d = time.Second

		c <- GatewayMsg{Type: Connect}
		a.ws = conn
		a.wsLock.Unlock()

		a.gwConnected(ctx, conn, dev, port)
		err = a.gwReadLoop(ctx, conn, c)

		a.wsLock.Lock()
		c <- GatewayMsg{Type: Disconnect, Delay: d, Error: err}
		t.Reset(d)

	wait:
		select {
		case <-ctx.Done():
			// Read loop can exit because of the context.
			if conn != nil {
				conn.Close(websocket.StatusNormalClosure, "closing")
			}
			return
		case <-t.C:
			continue
		}
	}
}

func (a *API) GatewaySend(ctx context.Context, msg GatewayMsg) error {
	a.wsLock.Lock()
	defer a.wsLock.Unlock()

	return wsjson.Write(ctx, a.ws, msg)
}
