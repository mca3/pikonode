package discov

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestNewHello(t *testing.T) {
	buf := NewHello(0xdead, "BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAC", false)

	if !bytes.Equal(buf[:4], []byte("PIKO")) {
		t.Error("header missing")
	}

	if buf[4] != Hello {
		t.Errorf("wrong command: expected %d, got %d", Hello, buf[4])
	}

	if !bytes.Equal(buf[5:7], []byte{0xde, 0xad}) {
		t.Errorf("invalid port: expected %v, got %v", []byte{0xde, 0xad}, buf[5:7])
	}

	// This all works the same so we only check the one value that actually
	// changes.
	buf = NewHello(0xdead, "BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAC", true)

	if buf[4] != HelloReply {
		t.Errorf("wrong command: expected %d, got %d", HelloReply, buf[4])
	}
}

func TestDiscov(t *testing.T) {
	// --- Some notes about this test ---
	//
	// This test is a mess because I tossed sync into the mix so I wouldn't
	// see races in my tests, even though it really doesn't matter.
	//
	// Additionally we use the actual network for this, so we may spam the
	// network with garbage. You need a network connection to run this
	// test.
	//
	// Everyone's test coverage is going to be slightly different because
	// of isGoodInterfaces(). Just pretend it's 100%.

	if ifs, err := fetchInterfaces(); err != nil || len(ifs) == 0 {
		t.Skipf("failed to fetch interfaces or no suitable interfaces were found. len(ifs) = %d, err = %v", len(ifs), err)
		return
	}

	d := Discovery{Ready: make(chan struct{})}
	mu := sync.Mutex{}
	flag := false

	// The timeout shouldn't be a problem and exists solely to control how
	// long we're willing to wait before the test is considered "failed."
	// Don't run this test on shoddy networks, or if you really have to,
	// increase the timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	d.Notify = func(_ *net.UDPAddr, m Message) {
		// Set the flag so we can say that the test has passed.
		mu.Lock()
		flag = true
		mu.Unlock()

		// Cancel the context to stop sending messages.
		cancel()
	}

	// We need to Listen and receive messages, but Listen blocks.
	// We can't check for an error immediately either because there is no
	// guarantee that a goroutine executes immediately, but it is
	// guaranteed it will be executed "eventually."
	errCh := make(chan error)
	go func() {
		errCh <- d.Listen(ctx)
	}()
	defer close(errCh)

	// Wait for the signal
	select {
	case <-d.Ready:
	case err := <-errCh:
		// If we end up here, the listen failed so fail the
		// test.
		t.Fatalf("failed to listen: %v", err)
	}

	msg := NewHello(12345, "BAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAC", false)
	d.Send(msg)

	// Wait until we timeout or the context is cancelled.
	<-ctx.Done()

	mu.Lock()
	if !flag {
		t.Fatal("timeout")
	}
}
