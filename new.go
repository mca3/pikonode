package main

import (
	"context"
	"os"
)

func cmdNew(args []string) {
	// I'm sorry.
	if len(args) == 0 {
		die(`usage: %s new device <name> <public key>
       %s new network <name>`, os.Args[0], os.Args[0])
	}

	switch args[0] {
	case "device", "dev", "d":
		if _, err := rv.NewDevice(context.Background(), args[1], args[2]); err != nil {
			die("couldn't add device: %v", err)
		}
	case "network", "net", "nw":
		if _, err := rv.NewNetwork(context.Background(), args[1]); err != nil {
			die("couldn't add network: %v", err)
		}
	default:
		die("can only make new devices and networks")
	}
}
