.PHONY: build build-fast frontend run install update update-fast clean \
        restart refresh status park forget link unlink paths links logs daemon

BINARY_NAME=sld
MAIN_PATH=./cmd/sld

# ─────────────────────────────────────────────
#  Build
# ─────────────────────────────────────────────

# Full rebuild (frontend + backend) then install
update: build
	@echo "Installing $(BINARY_NAME)..."
	sudo ./$(BINARY_NAME) install
	@echo "Done! SLD updated successfully. 🚀"

# Backend-only rebuild then install (faster — skips npm)
update-fast: build-fast
	@echo "Installing $(BINARY_NAME) (fast)..."
	sudo ./$(BINARY_NAME) install
	@echo "Done! SLD updated successfully. 🚀"

# Build binary with frontend
build: frontend
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete."

# Build backend only — skips npm
build-fast:
	@echo "Building $(BINARY_NAME) (backend only)..."
	go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete."

# Build frontend assets only
frontend:
	@echo "Building frontend..."
	cd sld-dashboard && npm install && npm run build
	@echo "Updating embedded assets..."
	rm -rf pkg/assets/gui/*
	cp -r sld-dashboard/dist/* pkg/assets/gui/
	@echo "Frontend build complete."

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)

# Install to GOPATH (dev use)
install:
	go install $(MAIN_PATH)

# Build and run the daemon (foreground)
run: build
	@echo "Starting daemon..."
	./$(BINARY_NAME) daemon

# ─────────────────────────────────────────────
#  SLD System Commands  (proxy to running daemon)
# ─────────────────────────────────────────────

# Show service status
status:
	sld status

# Restart all services (Nginx, PHP-FPM, Dnsmasq)
restart:
	sld restart

# Re-scan all parked directories for project changes
refresh:
	sld refresh

# Start daemon in foreground (for debugging)
daemon:
	sudo sld daemon

# Park a directory. Usage: make park path=/home/you/projects
park:
	@if [ -z "$(path)" ]; then echo "Usage: make park path=<directory>"; exit 1; fi
	sld park $(path)

# Un-park a directory. Usage: make forget path=/home/you/projects
forget:
	@if [ -z "$(path)" ]; then echo "Usage: make forget path=<directory>"; exit 1; fi
	sld forget $(path)

# Link current directory to a .test domain. Usage: make link name=myapp
link:
	@if [ -z "$(name)" ]; then echo "Usage: make link name=<sitename>"; exit 1; fi
	sld link $(name)

# Unlink a site. Usage: make unlink name=myapp
unlink:
	@if [ -z "$(name)" ]; then echo "Usage: make unlink name=<sitename>"; exit 1; fi
	sld unlink $(name)

# List all parked directories
paths:
	sld paths

# List all linked sites
links:
	sld links

# Tail nginx error logs (default). Usage: make logs svc=nginx-access
logs:
	sld logs $(or $(svc),nginx-error)
