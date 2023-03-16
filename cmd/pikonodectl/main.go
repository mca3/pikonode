package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/internal/config"
)

var rv *api.API

func die(f string, d ...any) {
	fmt.Fprintf(os.Stderr, f+"\n", d...)
	os.Exit(1)
}

func ping(args []string) {
	if len(args) < 1 {
		die("usage: %s ping [device id] <port>", os.Args[0])
	}

	did := int(config.Cfg.DeviceID)

	if len(args) >= 2 {
		var err error
		did, err = strconv.Atoi(args[0])
		if err != nil {
			die("supply a valid numeric device id")
		}
		args = args[1:]
	}

	port, err := strconv.Atoi(args[0])
	if err != nil || port > 65536 || port <= 0 {
		die("supply a valid port number")
	}

	if err := rv.Ping(context.Background(), int64(did), port); err != nil {
		die("couldn't ping: %v", err)
	}
}

func login(args []string) {
	if len(args) < 2 {
		die("usage: %s login <username> <password>", os.Args[0])
	}

	if err := rv.Login(context.Background(), args[0], args[1]); err != nil {
		die("failed to login: %v", err)
	}

	fmt.Printf("token: %v\n", rv.Token)

	config.Cfg.Token = rv.Token
	if err := config.SaveConfigFile(); err != nil {
		die("failed to save config: %v", err)
	}
}

func main() {
	if err := config.ReadConfigFile(); err != nil {
		die("failed to read config file: %v", err)
	}

	if len(os.Args) < 2 {
		fmt.Printf(strings.ReplaceAll(`pikonode

%s login <username> <password>
	login to the rendezvous server

%s list {networks,devices}
	list networks or devices attached to your account

%s new device <name> <pubkey>
	create a new device

%s new network <name>
	create a new network

%s join <device id> <network id>
	add a device to a network

%s leave <device id> <network id>
	remove a device from a network

%s ping [device id] <port>
	update a/your device's endpoint
`, "%s", os.Args[0]))
		return
	}

	rv = &api.API{
		Server: config.Cfg.Rendezvous,
		Token:  config.Cfg.Token,
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
		cmdNew(os.Args[2:])
	case "join":
		join(os.Args[2:])
	case "leave":
		leave(os.Args[2:])
	case "ping":
		ping(os.Args[2:])
	}
}
