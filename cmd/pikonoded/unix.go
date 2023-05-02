package main

import (
	"context"
	"net"
	/*
		"errors"
		"log"
	*/)

const (
	unixMode = 0o770
)

var unixSocket = ""

func handle(c net.Conn) {
	defer waitGroup.Done()
}

// bindUnix creates the UNIX socket that allows programs to communicate with
// this daemon.
func bindUnix(ctx context.Context) error {
	/*
		lc := net.ListenConfig{}

		l, err := lc.Listen(ctx, "unix", unixSocket)
		if err != nil {
			return err
		}

		waitGroup.Add(2)

		go func() {
			defer waitGroup.Done()

			<-ctx.Done()
			l.Close()
		}()

		go func() {
			defer waitGroup.Done()
			defer l.Close()

			for {
				c, err := l.Accept()
				if errors.Is(err, net.ErrClosed) {
					// Context is done
					return
				} else if err != nil {
					log.Printf("Couldn't accept: %v", err)
					return
				}

				waitGroup.Add(1)
				go handle(c)
			}
		}()

		log.Printf("UNIX socket is at %v", unixSocket)
	*/

	return nil
}
