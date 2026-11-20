.PHONY: build build-fast frontend run install update update-fast clean \
        restart refresh status park forget link unlink paths links logs daemon \
        docker-config docker-up docker-up-pg docker-app docker-stop docker-down docker-logs docker-build docker-ps docker-shell docker-clean \
        docker-images docker-image-app docker-image-router docker-image-nginx docker-image-php \n        test serve docker-token e2e

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

# Run the engine tests
test:
	go test ./internal/...

# End-to-end test of an SLD image against fixture projects (uses its own router
# and network, so it can run next to a real SLD). Usage: make e2e [SLD_IMAGE=sld:local]
e2e:
	SLD_IMAGE=$(or $(SLD_IMAGE),sld:local) bash scripts/e2e.sh

# Build and run the V2 control plane (dashboard + API with token auth) in the foreground
serve: build
	./$(BINARY_NAME) serve

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

# ─────────────────────────────────────────────
#  Docker (dashboard + MariaDB + phpMyAdmin + Redis + Mailpit)
# ─────────────────────────────────────────────

docker-config:
	docker compose config -q

docker-build:
	docker compose build

docker-up:
	docker compose up --build -d

docker-up-pg:
	docker compose --profile postgres up --build -d

# Start only the SLD dashboard/control plane (useful when host ports 3306/6379 are used by other projects)
docker-app:
	docker compose up --build -d --no-deps app

docker-stop:
	docker compose stop

docker-down:
	docker compose down

docker-ps:
	docker compose ps

docker-logs:
	docker compose logs -f --tail=100

docker-shell:
	docker compose exec app sh

# WARNING: Destructive — deletes persistent sld-mariadb, sld-postgres, sld-redis, and sld-state volumes!
docker-clean:
	docker compose down -v

# Show the admin token printed by the control plane on first start
docker-token:
	@docker compose logs app 2>&1 | grep -A2 "admin token" | tail -1 || echo "No token in the logs: restart with 'docker compose run --rm app serve --reset-token'"

# ─────────────────────────────────────────────
#  V2 Official Docker Images
# ─────────────────────────────────────────────

docker-images: docker-image-app docker-image-router docker-image-nginx docker-image-php

docker-image-app:
	docker build -t ghcr.io/supreme-majesty/sld:latest .

docker-image-router:
	docker build -t ghcr.io/supreme-majesty/sld-router:2 -t ghcr.io/supreme-majesty/sld-router:latest docker/router

docker-image-nginx:
	docker build -t ghcr.io/supreme-majesty/sld-nginx:1 -t ghcr.io/supreme-majesty/sld-nginx:latest docker/nginx

docker-image-php:
	docker build --build-arg PHP_VERSION=8.1 -t ghcr.io/supreme-majesty/sld-php:8.1 docker/php
	docker build --build-arg PHP_VERSION=8.2 -t ghcr.io/supreme-majesty/sld-php:8.2 docker/php
	docker build --build-arg PHP_VERSION=8.3 -t ghcr.io/supreme-majesty/sld-php:8.3 docker/php
	docker build --build-arg PHP_VERSION=8.4 -t ghcr.io/supreme-majesty/sld-php:8.4 docker/php

