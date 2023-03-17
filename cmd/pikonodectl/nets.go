package main

import (
	"context"
	"os"
	"strconv"
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
