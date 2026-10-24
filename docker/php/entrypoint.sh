#!/bin/sh
set -e

WWWUSER=${WWWUSER:-1000}
WWWGROUP=${WWWGROUP:-1000}

# Update www-data group GID if specified and different
if [ "$WWWGROUP" -ne 0 ] && [ "$WWWGROUP" -ne "$(id -g www-data 2>/dev/null)" ]; then
    groupmod -o -g "$WWWGROUP" www-data 2>/dev/null || true
fi

# Update www-data user UID if specified and different
if [ "$WWWUSER" -ne 0 ] && [ "$WWWUSER" -ne "$(id -u www-data 2>/dev/null)" ]; then
    usermod -o -u "$WWWUSER" -g "$WWWGROUP" www-data 2>/dev/null || true
fi

# Ensure storage / app permissions if running as root
if [ "$(id -u)" -eq 0 ]; then
    # If executing CLI commands (artisan, composer, bash), step down to www-data
    if [ "$1" != "php-fpm" ]; then
        exec gosu www-data "$@"
    fi
fi

exec "$@"
