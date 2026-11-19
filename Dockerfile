# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS frontend
WORKDIR /src/sld-dashboard
COPY sld-dashboard/package.json sld-dashboard/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY sld-dashboard/ ./
RUN npx vite build

FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS backend
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN rm -rf ./pkg/assets/gui/*
COPY --from=frontend /src/sld-dashboard/dist ./pkg/assets/gui
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-s -w -X main.Version=${VERSION}" -o /out/sld ./cmd/sld

FROM alpine:3.21
LABEL org.opencontainers.image.title="Supreme Local Dev (SLD)" \
      org.opencontainers.image.description="Local development dashboard, control plane, and database manager" \
      org.opencontainers.image.source="https://github.com/supreme-majesty/supreme-local-dev"

RUN apk add --no-cache ca-certificates curl mariadb-client mariadb-connector-c postgresql-client tzdata \
    && printf "[client]\nssl-verify-server-cert=0\n" > /etc/my.cnf.d/sld-client.cnf \
    && addgroup -g 10001 -S sld \
    && adduser -u 10001 -S -G sld -h /home/sld sld \
    && mkdir -p /var/lib/sld /home/sld/.sld \
    && chown -R sld:sld /var/lib/sld /home/sld

COPY --from=backend /out/sld /usr/local/bin/sld

ENV SLD_SKIP_INSTALL=1 \
    SLD_STATE_DIR=/home/sld/.sld

USER sld
WORKDIR /home/sld
EXPOSE 2025
STOPSIGNAL SIGTERM

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
    CMD curl -fsS http://127.0.0.1:2025/api/health || exit 1

ENTRYPOINT ["sld"]
CMD ["daemon"]
