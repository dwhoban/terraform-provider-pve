default: build

build:
	go build -v ./...

# Build into $(GOPATH)/bin (not GOBIN, which may sit inside a versioned Go
# toolchain dir) so a ~/.terraformrc dev_overrides entry stays stable.
install: build
	go build -v -o $(shell go env GOPATH)/bin/terraform-provider-pve .

lint:
	golangci-lint run

generate: gen
	cd tools; go generate ./...

# Regenerate the provider registry (internal/provider/registry.json), the
# ROADMAP Appendix A coverage table, and per-type examples from the API
# schema + phase mappings. The audit gates emission on full coverage.
gen:
	go run ./gen

fmt:
	gofmt -s -w -e .

test:
	go test -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: fmt lint test testacc build install generate gen
