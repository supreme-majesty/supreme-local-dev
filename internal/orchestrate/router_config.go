package orchestrate

// RouterStaticConfig is the router's Traefik static configuration. SLD writes
// it to its data directory and starts the router with --configFile, so it
// can change without publishing a new router image (docker/router/traefik.yml
// is the image's built-in copy and is kept the same).
//
// The access log goes to stdout as JSON, one line per request; X-Ray streams
// it from the router container. Only the User-Agent header is kept.
const RouterStaticConfig = `entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"

providers:
  docker:
    exposedByDefault: false
    network: "sld-global"
    watch: true
  file:
    directory: "/etc/traefik/dynamic"
    watch: true

api:
  dashboard: false

ping: {}

accessLog:
  format: json
  fields:
    defaultMode: keep
    headers:
      defaultMode: drop
      names:
        User-Agent: keep
`
