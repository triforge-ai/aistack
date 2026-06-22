BINARY := bin/ai
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# On macOS (Darwin 25) with older Go toolchains, the internal linker emits a
# binary missing LC_UUID that dyld refuses to run. Build with the external
# linker and ad-hoc codesign. On Linux / newer Go this is harmless.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
GOFLAGS := CGO_ENABLED=1
LDFLAGS := -ldflags "-linkmode=external -X main.version=$(VERSION)"
else
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
endif

.PHONY: build test vet db-up db-down db-logs release-snapshot clean

build:
	$(GOFLAGS) go build $(LDFLAGS) -o $(BINARY) ./cmd/ai
ifeq ($(UNAME_S),Darwin)
	@codesign -f -s - $(BINARY) 2>/dev/null || true
endif

test:
	$(GOFLAGS) go test $(LDFLAGS) ./...

vet:
	go vet ./...

# Start / stop the pgvector database (see docker-compose.yml).
db-up:
	docker compose up -d

db-down:
	docker compose down

db-logs:
	docker compose logs -f

# Build release artifacts locally without publishing (sanity check).
release-snapshot:
	goreleaser release --snapshot --clean --skip=publish

clean:
	rm -rf bin dist
