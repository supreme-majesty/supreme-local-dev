.PHONY: build build-fast frontend run install update clean

BINARY_NAME=sld
MAIN_PATH=./cmd/sld

# Build the binary (includes frontend rebuild)
build: frontend
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete."

# Build backend only — skips npm (faster for backend-only changes)
build-fast:
	@echo "Building $(BINARY_NAME) (backend only)..."
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

# Build + install system-wide in one command.
# Use 'make update' after pulling code changes.
# Use 'make update-fast' if you only changed Go backend code.
update: build
	@echo "Installing $(BINARY_NAME)..."
	sudo ./$(BINARY_NAME) install
	@echo "Done! SLD updated successfully. 🚀"

update-fast: build-fast
	@echo "Installing $(BINARY_NAME) (fast)..."
	sudo ./$(BINARY_NAME) install
	@echo "Done! SLD updated successfully. 🚀"

# Install to GOPATH (dev use)
install:
	go install $(MAIN_PATH)

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
