package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type httpError int
type Endpoint string

// API handles API calls.
type API struct {
	// Server is where the Pikonet Rendezvous server is hosted.
	// This must not change after the first call.
	Server string

	// Token is the Rendezvous token we receive after logging in.
	// This must be set, or will be set with a call to Login.
	//
	// Token must not change once set.
	Token string

	HTTP *http.Client
}

const (
	EndpointLogin        Endpoint = "/auth"
	EndpointListDevices  Endpoint = "/list/devices"
	EndpointListNetworks Endpoint = "/list/networks"
)

func (h httpError) Error() string {
	return fmt.Sprintf("http status code %3d", h)
}

func readError(res *http.Response) error {
	return httpError(res.StatusCode)
}

func (a *API) Endpoint(ep Endpoint) string {
	// TODO: Implement properly
	return a.Server + string(ep)
}

func encodeJSON(data any) (io.Reader, error) {
	b := &bytes.Buffer{}
	if err := json.NewEncoder(b).Encode(data); err != nil {
		return nil, err
	}
	return b, nil
}

// abuses generics to generate code for responses that just return JSON data
func makeGetJSONResp[T any](ep Endpoint) func(a *API, ctx context.Context) (T, error) {
	return func(a *API, ctx context.Context) (T, error) {
		var data T

		req, err := http.NewRequestWithContext(ctx, "GET", a.Endpoint(ep), nil)
		if err != nil {
			return data, err
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+a.Token)

		res, err := a.HTTP.Do(req)
		if err != nil {
			return data, err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			return data, readError(res)
		}

		err = json.NewDecoder(res.Body).Decode(&data)
		return data, err
	}
}

// Login logs into the Rendezvous server using username-password authentication
// and sets the token if successful.
func (a *API) Login(ctx context.Context, username, password string) error {
	body, err := encodeJSON(struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Method   string `json:"method"`
	}{username, password, "username-password"})
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.Endpoint(EndpointLogin), body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := a.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return readError(res)
	}

	// TODO: Login isn't implemented properly on rv yet

	token, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	a.Token = string(token)
	return nil
}

var (
	listDevices  = makeGetJSONResp[[]Device](EndpointListDevices)
	listNetworks = makeGetJSONResp[[]Network](EndpointListNetworks)
)

// Devices returns a list of devices attached to your user.
func (a *API) Devices(ctx context.Context) ([]Device, error) { return listDevices(a, ctx) }

// Network returns a list of networks attached to your user.
func (a *API) Networks(ctx context.Context) ([]Network, error) { return listNetworks(a, ctx) }
