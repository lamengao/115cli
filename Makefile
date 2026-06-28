BINARY := 115cli
BIN_DIR := $(HOME)/bin
BIN_PATH := $(BIN_DIR)/$(BINARY)

.PHONY: build install test clean

build:
	go build -o $(BINARY) .

install:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_PATH) .

test:
	go test ./...

clean:
	rm -f $(BINARY)
	rm -f $(BIN_PATH)
