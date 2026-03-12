.PHONY: help build-all build-controller build-connector build-agent build-frontend
.PHONY: dev-controller dev-connector dev-agent dev-frontend
.PHONY: test-all test-controller test-connector test-agent test-frontend
.PHONY: clean clean-all
.PHONY: build-ztna-client dev-ztna-client

help:
	@echo "ZTNA Project - Development Commands"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build-all        - Build all components"
	@echo "  make build-controller - Build controller (Go)"
	@echo "  make build-connector  - Build connector (Rust)"
	@echo "  make build-agent      - Build agent (Rust)"
	@echo "  make build-frontend   - Build frontend (React)"
	@echo ""
	@echo "Development Commands:"
	@echo "  make dev-controller   - Run controller in dev mode"
	@echo "  make dev-connector    - Run connector in dev mode"
	@echo "  make dev-agent        - Run agent in dev mode"
	@echo "  make dev-frontend     - Run frontend in dev mode"
	@echo ""
	@echo "Test Commands:"
	@echo "  make test-all         - Test all components"
	@echo "  make test-controller  - Test controller"
	@echo "  make test-connector   - Test connector"
	@echo "  make test-agent       - Test agent"
	@echo "  make test-frontend    - Test frontend"
	@echo ""
	@echo "Clean Commands:"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make clean-all        - Clean everything including deps"

# Build Commands
build-all: build-controller build-connector build-agent build-ztna-client build-frontend

build-controller:
	@echo "Building controller..."
	cd services/controller && go build -o ../../dist/controller .

CARGO ?= $(shell which cargo 2>/dev/null || echo /home/$(SUDO_USER)/.cargo/bin/cargo)

build-connector:
	@echo "Building connector..."
	cd services/connector && $(CARGO) build --release
	mkdir -p dist
	cp services/connector/target/release/connector dist/

build-agent:
	@echo "Building agent..."
	cd services/agent && $(CARGO) build --release
	mkdir -p dist
	cp services/agent/target/release/agent dist/

build-ztna-client:
	@echo "Building ztna-client..."
	cd services/ztna-client && cargo build --release
	mkdir -p dist
	cp services/ztna-client/target/release/ztna-client dist/

build-frontend:
	@echo "Building frontend..."
	cd apps/frontend && npm run build

# Development Commands
dev-controller:
	@echo "Running controller in dev mode..."
	cd services/controller && export $$(cat .env | grep -v '^#' | xargs) && \
		INTERNAL_CA_CERT="$$(cat ca/ca.crt)" INTERNAL_CA_KEY="$$(cat ca/ca.pkcs8.key)" go run .

dev-connector:
	@echo "Running connector in dev mode..."
	cd services/connector && cargo run

dev-agent:
	@echo "Running agent in dev mode..."
	cd services/agent && cargo run

dev-ztna-client:
	@echo "Running ztna-client in dev mode..."
	cd services/ztna-client && cargo run -- --controller-url http://localhost:8081

dev-frontend:
	@echo "Running frontend in dev mode..."
	cd apps/frontend && npm run dev

# Test Commands
test-all: test-controller test-connector test-agent test-frontend

test-controller:
	@echo "Testing controller..."
	cd services/controller && go test ./...

test-connector:
	@echo "Testing connector..."
	cd services/connector && cargo test

test-agent:
	@echo "Testing agent..."
	cd services/agent && cargo test

test-frontend:
	@echo "Testing frontend..."
	cd apps/frontend && npm test

# Clean Commands
clean:
	@echo "Cleaning build artifacts..."
	rm -rf dist/controller dist/connector dist/agent
	cd services/connector && cargo clean
	cd services/agent && cargo clean
	cd apps/frontend && rm -rf dist

clean-all: clean
	@echo "Cleaning all dependencies..."
	cd services/controller && rm -rf vendor
	cd apps/frontend && rm -rf node_modules
