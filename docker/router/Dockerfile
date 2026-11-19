FROM traefik:v3.1

COPY traefik.yml /etc/traefik/traefik.yml
RUN mkdir -p /etc/traefik/dynamic

EXPOSE 80 443

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD traefik healthcheck --ping || exit 1
