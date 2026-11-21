FROM traefik:v3.6
LABEL org.opencontainers.image.source="https://github.com/supreme-majesty/supreme-local-dev" \
      org.opencontainers.image.title="SLD router" \
      org.opencontainers.image.description="Traefik router for Supreme Local Dev projects"

COPY traefik.yml /etc/traefik/traefik.yml
RUN mkdir -p /etc/traefik/dynamic

EXPOSE 80 443

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD traefik healthcheck --ping || exit 1
