package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/mca3/pikonode/api"
)

var serverAddress = "http://localhost:8080/api"
var token = "1"
var rv *api.API

func die(f string, d ...any) {
	fmt.Fprintf(os.Stderr, f+"\n", d...)
	os.Exit(1)
}

func list(args []string) {
	switch args[0] {
	case "networks", "network", "nets", "net", "nws", "nw":
		nws, err := rv.Networks(context.Background())
		if err != nil {
			die("failed to list networks: %v", err)
		}

		for i, v := range nws {
			fmt.Printf("network id %d name \"%s\"\n", v.ID, v.Name)
			for _, v := range v.Devices {
				fmt.Printf("- device id %d name \"%s\"\n", v.ID, v.Name)
			}

			if i != len(nws)-1 {
				fmt.Println()
			}
		}
	case "devices", "device", "devs", "dev", "ds", "d":
		devs, err := rv.Devices(context.Background())
		if err != nil {
			die("failed to list devices: %v", err)
		}

		for i, v := range devs {
			fmt.Printf("device id %d name \"%s\"\n", v.ID, v.Name)
			for _, v := range v.Networks {
				fmt.Printf("- network id %d name \"%s\"\n", v.ID, v.Name)
			}

			if i != len(devs)-1 {
				fmt.Println()
			}
		}
	default:
		die("can list devices and networks")
	}
}

func login(args []string) {
	if err := rv.Login(context.Background(), args[0], args[1]); err != nil {
		die("failed to login: %v", err)
	}

	fmt.Printf("token: %v\n", rv.Token)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("subcommands: list {networks,devices}, login <username> <password>")
		return
	}

	sa := os.Getenv("PIKONET")
	if sa != "" {
		serverAddress = sa
	}

	rv = &api.API{
		Server: serverAddress,
		Token:  token,
		HTTP:   http.DefaultClient,
	}

	switch os.Args[1] {
	case "list", "ls":
		list(os.Args[2:])
	case "login":
		login(os.Args[2:])
	}
}
