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

# Run with live reload (air): rebuilds and restarts on changes, reloads the browser
run-live:
    go tool air
