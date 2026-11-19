#!/bin/sh
set -e

WWWUSER=${WWWUSER:-1000}
WWWGROUP=${WWWGROUP:-1000}

# Validate numeric UID/GID before modifying www-data
case "$WWWGROUP" in
    ''|*[!0-9]*) WWWGROUP=1000 ;;
esac
case "$WWWUSER" in
    ''|*[!0-9]*) WWWUSER=1000 ;;
esac

# Update www-data group GID if specified and different
if [ "$WWWGROUP" -ne 0 ] && [ "$WWWGROUP" -ne "$(id -g www-data 2>/dev/null || echo 0)" ]; then
    groupmod -o -g "$WWWGROUP" www-data 2>/dev/null || true
fi

# Update www-data user UID if specified and different
if [ "$WWWUSER" -ne 0 ] && [ "$WWWUSER" -ne "$(id -u www-data 2>/dev/null || echo 0)" ]; then
    usermod -o -u "$WWWUSER" -g "$WWWGROUP" www-data 2>/dev/null || true
fi

# If running as root for CLI commands (artisan, composer, bash), step down to www-data
if [ "$(id -u)" -eq 0 ]; then
    if [ "$1" != "php-fpm" ]; then
        exec gosu www-data "$@"
    fi
fi

exec "$@"
