.PHONY: build build-fast frontend install clean test e2e serve \
        start stop status scan doctor \
        docker-config docker-up docker-up-pg docker-app docker-stop docker-down docker-logs docker-build docker-ps docker-shell docker-clean \
        docker-images docker-image-app docker-image-router docker-image-nginx docker-image-php docker-publish token

BINARY_NAME=sld
MAIN_PATH=./cmd/sld

# Where SLD's images are published (Docker Hub namespace). SLD itself uses the
# same default; override both with SLD_IMAGE_REGISTRY / IMAGE_REGISTRY.
IMAGE_REGISTRY ?= insightifytech
VERSION ?= dev
PLATFORMS ?= linux/amd64,linux/arm64

# ─────────────────────────────────────────────
#  Build
# ─────────────────────────────────────────────

# Build binary with the dashboard embedded
build: frontend
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete."

# Build backend only — skips npm (embeds whatever is in pkg/assets/gui)
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

# Run the Go tests
test:
	go test ./...

# End-to-end test of an SLD image against fixture projects (uses its own router
# and network, so it can run next to a real SLD). Usage: make e2e [SLD_IMAGE=sld:local]
e2e:
	SLD_IMAGE=$(or $(SLD_IMAGE),sld:local) bash scripts/e2e.sh

# Build and run the control plane (dashboard + API with token auth) in the foreground
serve: build
	./$(BINARY_NAME) serve

# ─────────────────────────────────────────────
#  Projects without the SLD container (run inside a project folder)
# ─────────────────────────────────────────────

start:
	sld start

stop:
	sld stop

status:
	sld status --all

# Find projects under a folder. Usage: make scan [dir=~/Developments]
scan:
	sld scan $(or $(dir),.)

doctor:
	sld doctor

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

# Print a new admin token and a sign-in link (the running dashboard accepts it at once)
token:
	docker compose exec app sld token


# ─────────────────────────────────────────────
#  V2 Official Docker Images
# ─────────────────────────────────────────────

docker-images: docker-image-app docker-image-router docker-image-nginx docker-image-php

docker-image-app:
	docker build -t $(IMAGE_REGISTRY)/sld:latest .

docker-image-router:
	docker build -t $(IMAGE_REGISTRY)/sld-router:2 -t $(IMAGE_REGISTRY)/sld-router:latest docker/router

docker-image-nginx:
	docker build -t $(IMAGE_REGISTRY)/sld-nginx:1 -t $(IMAGE_REGISTRY)/sld-nginx:latest docker/nginx

docker-image-php:
	docker build --build-arg PHP_VERSION=8.1 -t $(IMAGE_REGISTRY)/sld-php:8.1 docker/php
	docker build --build-arg PHP_VERSION=8.2 -t $(IMAGE_REGISTRY)/sld-php:8.2 docker/php
	docker build --build-arg PHP_VERSION=8.3 -t $(IMAGE_REGISTRY)/sld-php:8.3 docker/php
	docker build --build-arg PHP_VERSION=8.4 -t $(IMAGE_REGISTRY)/sld-php:8.4 docker/php


# Build all images for amd64 + arm64 and push them (needs `docker login` and a
# buildx builder that can build both platforms, e.g. Docker Desktop's).
#   make docker-publish VERSION=v3.0.0
docker-publish:
	docker buildx build --platform $(PLATFORMS) --build-arg VERSION=$(VERSION) \
		-t $(IMAGE_REGISTRY)/sld:latest $(if $(filter-out dev,$(VERSION)),-t $(IMAGE_REGISTRY)/sld:$(VERSION)) --push .
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE_REGISTRY)/sld-router:2 -t $(IMAGE_REGISTRY)/sld-router:latest --push docker/router
	docker buildx build --platform $(PLATFORMS) -t $(IMAGE_REGISTRY)/sld-nginx:1 -t $(IMAGE_REGISTRY)/sld-nginx:latest --push docker/nginx
	for v in 8.1 8.2 8.3 8.4; do \
		docker buildx build --platform $(PLATFORMS) --build-arg PHP_VERSION=$$v -t $(IMAGE_REGISTRY)/sld-php:$$v --push docker/php || exit 1; \
	done
