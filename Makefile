OPENAPI_SOURCE ?= ../omnimemory/router/docs/openapi.v2.json

.PHONY: snapshot generate check-generated test coverage-v4

snapshot:
	./scripts/extract-data-plane-openapi.sh "$(OPENAPI_SOURCE)"

generate:
	go generate ./internal/generated

check-generated: generate
	git diff --exit-code -- internal/generated/ogen

test:
	go test ./...

coverage-v4:
	@profile="$$(mktemp)"; \
	go test -coverprofile="$$profile" . >/dev/null; \
	awk '/\/v4.go:/ { total += $$2; if ($$3 > 0) covered += $$2 } END { printf "v4.go statements: %.1f%% (%d/%d)\n", 100 * covered / total, covered, total; if (covered != total) exit 1 }' "$$profile"
