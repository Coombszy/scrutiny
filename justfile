# List recipes
default:
    @just --list

# Generate Templ components
generate:
    go tool templ generate

# Build the scrutiny binary into ./bin
build: generate
    go build -o bin/scrutiny ./cmd/scrutiny

# Run the full test suite
test: generate
    go test ./...

# Vet the code
vet: generate
    go vet ./...

# Build and run `scrutiny serve`
run *args: build
    ./bin/scrutiny serve {{args}}

# Run with live reload (air) on a port (default 7878): rebuilds and restarts on changes, reloads the browser
run-live *args:
    #!/usr/bin/env bash
    set -euo pipefail
    port=7878
    set -- {{args}}
    while [ $# -gt 0 ]; do
        case "$1" in
            --port|-port) port="$2"; shift 2 ;;
            --port=*|-port=*) port="${1#*=}"; shift ;;
            *[!0-9]*) echo "usage: just run-live [--port N]" >&2; exit 2 ;;
            *) port="$1"; shift ;;
        esac
    done
    # air's proxy (which reloads the browser) takes the port; Scrutiny runs on the next one.
    app=$((port + 1))
    exec go tool air --proxy.proxy_port "$port" --proxy.app_port "$app" -- serve -dev -port "$app"
