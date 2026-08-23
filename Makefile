.PHONY: all build test eval vet clean help

BINARY_NAME := bin/verifoxx

all: build eval

build:
	@mkdir -p bin
	go build -o $(BINARY_NAME) ./cmd/verifoxx

test:
	go test -v -timeout 60s ./...

eval: build
	./$(BINARY_NAME) --policy policies/policy.json --requests fixtures/requests.json --evidence fixtures/evidence.json --output results/requests.json

vet:
	go vet ./...

clean:
	rm -rf bin/ results/requests.json

help:
	@echo "Verifoxx Build System Commands:"
	@echo "  make build   - Compile the verifoxx binary to bin/verifoxx"
	@echo "  make test    - Run unit tests with 60s timeout"
	@echo "  make eval    - Build and execute policy evaluation against requests R1-R5"
	@echo "  make vet     - Run go vet static analysis"
	@echo "  make clean   - Remove build artifacts and generated results"
	@echo "  make all     - Build and run evaluation"
