FROM nginx:1.27
LABEL org.opencontainers.image.source="https://github.com/supreme-majesty/supreme-local-dev" \
      org.opencontainers.image.title="SLD nginx" \
      org.opencontainers.image.description="nginx sidecar for Supreme Local Dev PHP and static apps"

RUN apt-get update \
    && apt-get install -y --no-install-recommends gettext-base wget \
    && rm -rf /var/lib/apt/lists/*

COPY default.conf.template /etc/nginx/templates/default.conf.template
COPY entrypoint.sh /usr/local/bin/sld-nginx-entrypoint
RUN sed -i 's/\r$//' /usr/local/bin/sld-nginx-entrypoint \
    && chmod +x /usr/local/bin/sld-nginx-entrypoint

EXPOSE 80

HEALTHCHECK --interval=5s --timeout=2s --start-period=3s --retries=3 \
    CMD wget -q -O - http://127.0.0.1:80/sld-health || exit 1

ENTRYPOINT ["/usr/local/bin/sld-nginx-entrypoint"]
CMD ["nginx", "-g", "daemon off;"]
