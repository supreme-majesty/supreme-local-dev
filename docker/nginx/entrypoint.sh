#!/bin/sh
set -e

# Default variables if unset
export WEB_ROOT="${WEB_ROOT:-public}"
export PHP_HOST="${PHP_HOST:-php}"

# Substitute WEB_ROOT and PHP_HOST in template
envsubst '${WEB_ROOT} ${PHP_HOST}' < /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf

exec "$@"
