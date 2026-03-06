.PHONY: help build-all build-controller build-connector build-tunneler build-frontend
.PHONY: dev-controller dev-connector dev-tunneler dev-frontend
.PHONY: test-all test-controller test-connector test-tunneler test-frontend
.PHONY: clean clean-all

help:
	@echo "ZTNA Project - Development Commands"
	@echo ""
	@echo "Build Commands:"
	@echo "  make build-all        - Build all components"
	@echo "  make build-controller - Build controller (Go)"
	@echo "  make build-connector  - Build connector (Rust)"
	@echo "  make build-tunneler   - Build tunneler (Rust)"
	@echo "  make build-frontend   - Build frontend (React)"
	@echo ""
	@echo "Development Commands:"
	@echo "  make dev-controller   - Run controller in dev mode"
	@echo "  make dev-connector    - Run connector in dev mode"
	@echo "  make dev-tunneler     - Run tunneler in dev mode"
	@echo "  make dev-frontend     - Run frontend in dev mode"
	@echo ""
	@echo "Test Commands:"
	@echo "  make test-all         - Test all components"
	@echo "  make test-controller  - Test controller"
	@echo "  make test-connector   - Test connector"
	@echo "  make test-tunneler    - Test tunneler"
	@echo "  make test-frontend    - Test frontend"
	@echo ""
	@echo "Clean Commands:"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make clean-all        - Clean everything including deps"

# Build Commands
build-all: build-controller build-connector build-tunneler build-frontend

build-controller:
	@echo "Building controller..."
	cd services/controller && go build -o ../../dist/controller .

build-connector:
	@echo "Building connector..."
	cd services/connector && cargo build --release
	mkdir -p dist
	cp services/connector/target/release/grpcconnector2 dist/

build-tunneler:
	@echo "Building tunneler..."
	cd services/tunneler && cargo build --release
	mkdir -p dist
	cp services/tunneler/target/release/grpctunneler dist/

build-frontend:
	@echo "Building frontend..."
	cd apps/frontend && npm run build

# Development Commands
dev-controller:
	@echo "Running controller in dev mode..."
	cd services/controller && go run .

dev-connector:
	@echo "Running connector in dev mode..."
	cd services/connector && cargo run

dev-tunneler:
	@echo "Running tunneler in dev mode..."
	cd services/tunneler && cargo run

dev-frontend:
	@echo "Running frontend in dev mode..."
	cd apps/frontend && npm run dev

# Test Commands
test-all: test-controller test-connector test-tunneler test-frontend

test-controller:
	@echo "Testing controller..."
	cd services/controller && go test ./...

test-connector:
	@echo "Testing connector..."
	cd services/connector && cargo test

test-tunneler:
	@echo "Testing tunneler..."
	cd services/tunneler && cargo test

test-frontend:
	@echo "Testing frontend..."
	cd apps/frontend && npm test

# Clean Commands
clean:
	@echo "Cleaning build artifacts..."
	rm -rf dist/controller dist/grpcconnector2 dist/grpctunneler
	cd services/connector && cargo clean
	cd services/tunneler && cargo clean
	cd apps/frontend && rm -rf dist

clean-all: clean
	@echo "Cleaning all dependencies..."
	cd services/controller && rm -rf vendor
	cd apps/frontend && rm -rf node_modules
