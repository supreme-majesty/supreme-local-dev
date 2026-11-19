ARG PHP_VERSION=8.3
FROM php:${PHP_VERSION}-fpm-bookworm

# Install mlocati/docker-php-extension-installer
ADD --chmod=0755 https://github.com/mlocati/docker-php-extension-installer/releases/latest/download/install-php-extensions /usr/local/bin/

# Install system dependencies, tools, and extensions
RUN apt-get update && apt-get install -y --no-install-recommends \
        git \
        curl \
        unzip \
        libfcgi-bin \
        gosu \
    && rm -rf /var/lib/apt/lists/* \
    && install-php-extensions \
        pdo_mysql \
        pdo_pgsql \
        redis \
        gd \
        zip \
        bcmath \
        intl \
        mbstring \
        opcache \
        pcntl \
        exif \
        soap \
        sockets \
        xdebug \
    && install-php-extensions @composer

# Configure PHP-FPM ping endpoint for container health checking
RUN { \
        echo "[www]"; \
        echo "ping.path = /ping"; \
        echo "ping.response = pong"; \
    } > /usr/local/etc/php-fpm.d/zz-sld-ping.conf

# Configure Xdebug (disabled by default, toggled via XDEBUG_MODE)
RUN { \
        echo "xdebug.mode=\${XDEBUG_MODE:-off}"; \
        echo "xdebug.start_with_request=yes"; \
        echo "xdebug.client_host=host.docker.internal"; \
    } > /usr/local/etc/php/conf.d/docker-php-ext-xdebug.ini

# Development PHP ini overrides
RUN { \
        echo "upload_max_filesize = 128M"; \
        echo "post_max_size = 128M"; \
        echo "memory_limit = 512M"; \
        echo "max_execution_time = 600"; \
        echo "variables_order = 'EGPCS'"; \
    } > /usr/local/etc/php/conf.d/sld-dev.ini

COPY entrypoint.sh /usr/local/bin/sld-php-entrypoint
COPY sld-php-healthcheck /usr/local/bin/sld-php-healthcheck
RUN sed -i 's/\r$//' /usr/local/bin/sld-php-entrypoint /usr/local/bin/sld-php-healthcheck \
    && chmod +x /usr/local/bin/sld-php-entrypoint /usr/local/bin/sld-php-healthcheck

WORKDIR /app

EXPOSE 9000

HEALTHCHECK --interval=5s --timeout=2s --start-period=5s --retries=3 \
    CMD /usr/local/bin/sld-php-healthcheck || exit 1

ENTRYPOINT ["/usr/local/bin/sld-php-entrypoint"]
CMD ["php-fpm"]
