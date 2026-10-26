GOTOOLCHAIN ?= go1.26.10
export GOTOOLCHAIN
BUILD_DIR ?= /tmp/mxroute-provider-build
DOCS_VERSION := v0.25.0
VULN_VERSION := v1.7.0
RELEASE_VERSION ?= 0.1.0-beta.1-dev
RELEASE_DIR ?= /tmp/mxroute-provider-dist

.PHONY: build fmt fmt-check lint test vuln docs docs-check inventory check snapshot privacy release-smoke

build:
	mkdir -p $(BUILD_DIR)
	go build -trimpath -o $(BUILD_DIR)/terraform-provider-mxroute .

fmt:
	gofmt -w main.go internal cmd
	terraform fmt -recursive examples

fmt-check:
	test -z "$$(gofmt -l main.go internal cmd)"
	terraform fmt -check -recursive examples

lint:
	go vet ./...

test:
	go test -mod=readonly -race ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@$(VULN_VERSION) ./...

docs:
	python3 scripts/generate_docs.py --version $(DOCS_VERSION)

docs-check:
	python3 scripts/generate_docs.py --version $(DOCS_VERSION) --check

inventory:
	python3 scripts/api_inventory.py --check
	python3 -m unittest discover -s scripts -p 'test_*.py' -v

privacy:
	python3 scripts/privacy_check.py --worktree --history

check: fmt-check lint test inventory docs-check privacy

# Snapshot mode never uploads; the tag workflow publishes verified release assets.
snapshot:
	go run github.com/goreleaser/goreleaser/v2@v2.17.0 release --snapshot --clean --skip=sign
	cp terraform-registry-manifest.json $(RELEASE_DIR)/terraform-provider-mxroute_$(RELEASE_VERSION)_manifest.json
	python3 scripts/verify_release.py --directory $(RELEASE_DIR) --version $(RELEASE_VERSION) --unsigned-snapshot

release-smoke:
	test -n "$(MXROUTE_RELEASE_ARCHIVE)"
	go test -mod=readonly ./internal/provider -run '^TestOfflineReleasedProviderInstallation$$' -count=1 -v
