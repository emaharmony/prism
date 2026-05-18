.PHONY: dev test build build-all lint clean

# Development
dev:                           ## Run in development mode
	go run ./cmd/prism-cli run --task "hello world" --project dev --agent lumi

# Testing
test:                          ## Run all tests
	go test ./... -count=1 -race

test-coverage:                 ## Run tests with coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-short:                    ## Run tests without race detector (faster)
	go test ./... -count=1 -short

# Building
build:                         ## Build binary for current platform
	CGO_ENABLED=0 go build -o prism ./cmd/prism-cli/

build-all:                     ## Cross-compile for all platforms
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o prism-linux-amd64 ./cmd/prism-cli/
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o prism-darwin-arm64 ./cmd/prism-cli/
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o prism-windows-amd64.exe ./cmd/prism-cli/
	@echo "Built: prism-linux-amd64, prism-darwin-arm64, prism-windows-amd64.exe"

# Linting
lint:                          ## Run linters
	go vet ./...
	@echo "vet: OK"

# Docker
docker-build:                  ## Build Docker image
	docker build -t prism:latest .

docker-run:                    ## Run in Docker
	docker-compose up -d

docker-stop:                   ## Stop Docker containers
	docker-compose down

# Clean
clean:                         ## Remove build artifacts
	rm -f prism prism-linux-amd64 prism-darwin-arm64 prism-windows-amd64.exe coverage.out coverage.html
	go clean ./...