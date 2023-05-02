all: pikonoded pikonodectl

pikonoded: $(shell find cmd/pikonoded api internal net piko -name "*.go" -type f)
	go build ./cmd/pikonoded
	doas setcap cap_net_admin=+ep pikonoded

pikonodectl: $(shell find cmd/pikonodectl api internal net piko -name "*.go" -type f)
	go build ./cmd/pikonodectl

.PHONY: clean

clean:
	rm -f pikonoded pikonodectl
