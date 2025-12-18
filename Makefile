.PHONY: build run install clean

BINARY_NAME=sld
MAIN_PATH=./cmd/sld

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete."

# Build and run the daemon
run: build
	@echo "Starting daemon..."
	./$(BINARY_NAME) daemon

# Install to GOPATH
install:
	go install $(MAIN_PATH)

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
