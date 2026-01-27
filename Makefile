.PHONY: build run install clean

BINARY_NAME=sld
MAIN_PATH=./cmd/sld

# Build the binary
build: frontend
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete."

# Build frontend
frontend:
	@echo "Building frontend..."
	cd sld-dashboard && npm install && npm run build
	@echo "Updating embedded assets..."
	rm -rf pkg/assets/gui/*
	cp -r sld-dashboard/dist/* pkg/assets/gui/
	@echo "Frontend build complete."

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
