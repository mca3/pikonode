package main

import (
	"context"
	"fmt"
	"os"
)

func listNetworks() {
	nws, err := rv.Networks(context.Background())
	if err != nil {
		die("failed to list networks: %v", err)
	}

	for i, v := range nws {
		fmt.Printf("network id %d name \"%s\"\n", v.ID, v.Name)
		for _, v := range v.Devices {
			fmt.Printf("- device id %d name \"%s\" (%s)\n", v.ID, v.Name, v.Endpoint)
		}

		if i != len(nws)-1 {
			fmt.Println()
		}
	}
}

func listDevices() {
	devs, err := rv.Devices(context.Background())
	if err != nil {
		die("failed to list devices: %v", err)
	}

	for i, v := range devs {
		fmt.Printf("device id %d name \"%s\" (%s)\n", v.ID, v.Name, v.Endpoint)
		for _, v := range v.Networks {
			fmt.Printf("- network id %d name \"%s\"\n", v.ID, v.Name)
		}

		if i != len(devs)-1 {
			fmt.Println()
		}
	}
}

func list(args []string) {
	if len(args) == 0 {
		die("usage: %s list {networks,devices}", os.Args[0])
	}

	switch args[0] {
	case "networks", "network", "nets", "net", "nws", "nw":
		listNetworks()
	case "devices", "device", "devs", "dev", "ds", "d":
		listDevices()
	default:
		die("can only list devices and networks")
	}
}
