OPENAPI_SOURCE ?= ../omnimemory/router/docs/openapi.v2.json

.PHONY: snapshot generate check-generated test

snapshot:
	./scripts/extract-data-plane-openapi.sh "$(OPENAPI_SOURCE)"

generate:
	go generate ./internal/generated

check-generated: generate
	git diff --exit-code -- internal/generated/ogen

test:
	go test ./...
