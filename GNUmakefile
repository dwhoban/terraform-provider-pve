default: fmt lint install generate

build:
	go build -v ./...

install: build
	go install -v ./...

lint: bin/custom-gcl
	bin/custom-gcl run

bin/custom-gcl: .custom-gcl.yml
	@mkdir -p bin
	golangci-lint custom

generate:
	cd tools; go generate -tags generate ./...

validate-docs:
	cd tools; go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs validate --provider-dir .. -provider-name pve

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

.PHONY: fmt lint test testacc build install generate validate-docs
