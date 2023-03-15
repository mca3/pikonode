package api

// User represents a rendezvous user.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name,omitempty"`
}

// Network represents a network to which devices connect to
type Network struct {
	ID    int64  `json:"id"`
	Owner int64  `json:"owner"`
	Name  string `json:"name"`

	Devices []Device `json:"devices"`
}

// Device represnets a device, its unique Pikonet IP, and its public key.
type Device struct {
	ID    int64  `json:"id"`
	Owner int64  `json:"owner"`
	Name  string `json:"name,omitempty"`

	// PublicKey is the WireGuard public key for this device.
	PublicKey string `json:"key"`

	// IP is the Pikonet IP, which likely means a random IP in the range
	// fd00::/32.
	// This IP is not routable by the Internet, and only by Pikonet nodes.
	IP string `json:"ip"`

	Networks []Network `json:"networks"`
}
