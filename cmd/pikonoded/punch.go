package main

import (
	"context"
	"fmt"
	"net"
	"time"
)

func fetchEndpoint(ctx context.Context, ppaddr string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	ourAddr, err := net.ResolveUDPAddr("udp6", fmt.Sprintf("[%s]:0", ourDevice.IP))
	if err != nil {
		return "", err
	}
	srvAddr, err := net.ResolveUDPAddr("udp6", ppaddr)
	if err != nil {
		return "", err
	}

	cli, err := net.DialUDP("udp6", ourAddr, srvAddr)
	if err != nil {
		return "", err
	}
	defer cli.Close()

	buf := make([]byte, 64)

	go func() {
		for {
			select {
			case <-time.After(time.Second * 5):
				cli.Write([]byte(nil))
			case <-ctx.Done():
				cli.Close()
				return
			}
		}
	}()

	n, err := cli.Read(buf)
	if err != nil {
		return "", err
	}

	buf = buf[:n-1]
	return string(buf), nil
}
