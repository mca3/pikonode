package main

import (
	"context"
	"os"
	"strconv"

	"github.com/mca3/pikonode/api"
	"github.com/mca3/pikonode/internal/config"
)

func join(args []string) {
	if len(args) < 2 {
		die("usage: %s join <device id> <network id>", os.Args[0])
	}

	did, err := strconv.Atoi(args[0])
	if err != nil {
		die("supply a valid numeric device id")
	}

	nid, err := strconv.Atoi(args[1])
	if err != nil {
		die("supply a valid numeric network id")
	}

	if err := rv.JoinNetwork(context.Background(), int64(did), int64(nid)); err != nil {
		die("couldn't join network: %v", err)
	}
}

func leave(args []string) {
	if len(args) < 2 {
		die("usage: %s leave <device id> <network id>", os.Args[0])
	}

	did, err := strconv.Atoi(args[0])
	if err != nil {
		die("supply a valid numeric device id")
	}

	nid, err := strconv.Atoi(args[1])
	if err != nil {
		die("supply a valid numeric network id")
	}

	if err := rv.LeaveNetwork(context.Background(), int64(did), int64(nid)); err != nil {
		die("couldn't leave network: %v", err)
	}
}

func genconf(args []string) {
	if len(args) < 1 {
		die("usage: %s genconf <network id>", os.Args[0])
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		die("supply a valid numeric network id")
	}

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
		if v.ID == int64(config.Cfg.DeviceID) {
			us = v
		}
	}

	us.PrivateKey = config.Cfg.PrivateKey

	if nw.ID == 0 || us.ID == 0 {
		die("unknown network")
	}

	tmpl.ExecuteTemplate(os.Stdout, "wireguard", struct {
		api.Network
		Self api.Device
	}{Network: nw, Self: us})
}
