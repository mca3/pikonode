all: pikonoded pikonodectl

pikonoded: $(shell find cmd/pikonoded -name "*.go" -type f)
	go build ./cmd/pikonoded
	doas setcap cap_net_admin=+ep pikonoded

pikonodectl: $(shell find cmd/pikonoded -name "*.go" -type f)
	go build ./cmd/pikonodectl

.PHONY: clean

clean:
	rm -f pikonode
