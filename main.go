package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/mca3/pikonode/api"
)

var serverAddress = "http://localhost:8080/api"
var token = "1"
var rv *api.API
var ourDevid = 2
var ourPkey = ""
var ourPrivkey = ""

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

func newcmd(args []string) {
	switch args[0] {
	case "device", "dev", "d":
		if _, err := rv.NewDevice(context.Background(), args[1], args[2]); err != nil {
			die("couldn't add device: %v", err)
		}
	case "network", "net", "nw":
		if _, err := rv.NewNetwork(context.Background(), args[1]); err != nil {
			die("couldn't add network: %v", err)
		}
	}
}

func join(args []string) {
	did, _ := strconv.Atoi(args[0])
	nid, _ := strconv.Atoi(args[1])

	if err := rv.JoinNetwork(context.Background(), int64(did), int64(nid)); err != nil {
		die("couldn't join network: %v", err)
	}
}

func leave(args []string) {
	did, _ := strconv.Atoi(args[0])
	nid, _ := strconv.Atoi(args[1])

	if err := rv.LeaveNetwork(context.Background(), int64(did), int64(nid)); err != nil {
		die("couldn't leave network: %v", err)
	}
}

func genconf(args []string) {
	id, _ := strconv.Atoi(args[0])
	nws, err := rv.Networks(context.Background())
	var nw api.Network
	if err != nil {
		die("failed to list networks: %v", err)
	}

	for _, v := range nws {
		if v.ID == int64(id) {
			nw = v
			break
		}
	}

	var us api.Device

	for _, v := range nw.Devices {
		if v.ID == int64(ourDevid) {
			us = v
		}
	}

	us.PrivateKey = ourPrivkey

	if nw.ID == 0 || us.ID == 0 {
		panic("unknown network")
	}

	tmpl.ExecuteTemplate(os.Stdout, "wireguard", struct {
		api.Network
		Self api.Device
	}{Network: nw, Self: us})
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

	pk, err := os.ReadFile("pubkey")
	if err != nil {
		panic(err)
	}
	ourPkey = strings.TrimSuffix(string(pk), "\n")

	privk, err := os.ReadFile("privkey")
	if err != nil {
		panic(err)
	}
	ourPrivkey = strings.TrimSuffix(string(privk), "\n")

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
	case "genconf":
		genconf(os.Args[2:])
	case "new":
		newcmd(os.Args[2:])
	case "join":
		join(os.Args[2:])
	case "leave":
		leave(os.Args[2:])
	}
}
