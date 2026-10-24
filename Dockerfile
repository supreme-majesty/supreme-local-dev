FROM node:22-bookworm-slim AS frontend
WORKDIR /src/sld-dashboard
COPY sld-dashboard/package.json sld-dashboard/package-lock.json ./
RUN npm ci
COPY sld-dashboard/ ./
RUN npx vite build

FROM golang:1.25-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/sld-dashboard/dist ./pkg/assets/gui
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w -X main.Version=${VERSION}" -o /out/sld ./cmd/sld

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl mariadb-client \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --create-home --uid 10001 sld \
    && mkdir -p /var/lib/sld /home/sld/.sld \
    && chown -R sld:sld /var/lib/sld /home/sld
COPY --from=backend /out/sld /usr/local/bin/sld
ENV SLD_SKIP_INSTALL=1 \
    SLD_STATE_DIR=/home/sld/.sld
USER sld
WORKDIR /home/sld
EXPOSE 2025
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
    CMD curl -fsS http://127.0.0.1:2025/api/health || exit 1
ENTRYPOINT ["sld"]
CMD ["daemon"]
