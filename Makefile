# kmd — local CI. `make ci` runs every gate; any failure stops the pipeline.

GO          ?= go
BIN         := bin/kmd
PREFIX      ?= $(HOME)/.local
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X github.com/DipaolaFederico/kmd/internal/cli.Version=$(VERSION)

# Gates
COVER_MIN   ?= 90
PERF_MAX_MS ?= 10
PERF_RUNS   ?= 200
FUZZ_TIME   ?= 10s

# Dev tools, pinned and installed locally into .tools/ (never linked into kmd).
TOOLS               := $(CURDIR)/.tools/bin
STATICCHECK_VERSION := v0.8.2
GOVULNCHECK_VERSION := v1.10.0
GOSEC_VERSION       := v2.30.0

VERSION_RPM := $(shell sed -n 's/^Version: *//p' packaging/kmd.spec)
APPID       := io.github.dipaolafederico.kmd
DATADIR     ?= $(PREFIX)/share

.PHONY: ci fmt fmt-check lint vet staticcheck scripts-test test spec cover security vuln gosec \
        build perf fuzz bench install uninstall tools hooks clean packaging dist srpm rpm

ci: fmt-check lint scripts-test test cover security build packaging perf
	@printf '\n\033[1;32m✔ local CI passed\033[0m\n'

## Formatting ---------------------------------------------------------------
fmt:
	gofmt -s -w .

fmt-check:
	@echo '» fmt'
	@out=$$(gofmt -s -l .); if [ -n "$$out" ]; then echo "not gofmt'ed:"; echo "$$out"; exit 1; fi

## Lint ---------------------------------------------------------------------
lint: vet staticcheck

vet:
	@echo '» go vet'
	@$(GO) vet ./...

staticcheck: $(TOOLS)/staticcheck
	@echo '» staticcheck'
	@$(TOOLS)/staticcheck ./...

## Tests --------------------------------------------------------------------
scripts-test:
	@echo '» release scripts'
	@./scripts/test-scripts.sh

test:
	@echo '» test (race detector)'
	@$(GO) test -race -count=1 ./...

spec:
	@echo '» CommonMark 0.31.2 + GFM spec conformance'
	@$(GO) test -count=1 -run 'Spec' -v ./internal/md/ | grep -E 'examples pass'

cover:
	@echo '» coverage (min $(COVER_MIN)%)'
	@$(GO) test -count=1 -covermode=atomic -coverprofile=coverage.out ./internal/... >/dev/null
	@$(GO) tool cover -func=coverage.out | awk -v min=$(COVER_MIN) '/^total:/ { \
		sub("%", "", $$3); printf "  total coverage: %s%%\n", $$3; \
		if ($$3 + 0 < min) { print "  coverage below " min "%"; exit 1 } }'

fuzz:
	$(GO) test -run='^$$' -fuzz=FuzzParse -fuzztime=$(FUZZ_TIME) ./internal/md/
	$(GO) test -run='^$$' -fuzz=FuzzRender -fuzztime=$(FUZZ_TIME) ./internal/render/
	$(GO) test -run='^$$' -fuzz=FuzzMermaid -fuzztime=$(FUZZ_TIME) ./internal/diagram/
	$(GO) test -run='^$$' -fuzz=FuzzDisplay -fuzztime=$(FUZZ_TIME) ./internal/tex/
	$(GO) test -run='^$$' -fuzz=FuzzDataURI -fuzztime=$(FUZZ_TIME) ./internal/termimg/

bench:
	$(GO) test -run='^$$' -bench=. -benchmem ./internal/render/

## Security -----------------------------------------------------------------
security: vuln gosec

vuln: $(TOOLS)/govulncheck
	@echo '» govulncheck'
	@$(TOOLS)/govulncheck ./...

gosec: $(TOOLS)/gosec
	@echo '» gosec'
	@$(TOOLS)/gosec -quiet -fmt=text ./...

## Build & performance ------------------------------------------------------
build:
	@echo '» build'
	@CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) .
	@ls -lh $(BIN) | awk '{ print "  binary size: " $$5 }'

perf: build
	@echo '» perf (end-to-end, < $(PERF_MAX_MS) ms)'
	@./scripts/perf.sh $(BIN) testdata/sample.md $(PERF_MAX_MS) $(PERF_RUNS)

## Packaging (Fedora) -------------------------------------------------------
packaging: build
	@echo '» packaging metadata (man page, AppStream, completions, spec version)'
	@groff -man -Tutf8 -ww -z docs/kmd.1
	@appstreamcli validate --no-net --pedantic packaging/$(APPID).metainfo.xml >/dev/null
	@for sh in bash zsh fish; do $(BIN) --completion $$sh >/dev/null || exit 1; done
	@grep -q '"kmd $(VERSION_RPM)"' docs/kmd.1 || { echo "  docs/kmd.1 version != spec $(VERSION_RPM)"; exit 1; }
	@grep -q 'version="$(VERSION_RPM)"' packaging/$(APPID).metainfo.xml || { echo "  metainfo lacks release $(VERSION_RPM)"; exit 1; }
	@echo '  ok'

# Source tarball with the layout of a GitHub release archive (kmd-X.Y.Z/).
# Uses the files git tracks or would track (works before the first commit).
dist:
	@mkdir -p dist
	git ls-files -z --cached --others --exclude-standard | \
	  tar --null -T - --transform 's,^,kmd-$(VERSION_RPM)/,' --sort=name --owner=0 --group=0 \
	      --mtime='2026-01-01' -czf dist/kmd-$(VERSION_RPM).tar.gz
	@echo "dist/kmd-$(VERSION_RPM).tar.gz"

# Local RPM build (needs: sudo dnf install rpm-build go-rpm-macros libappstream-glib rpmlint).
srpm rpm: dist
	@mkdir -p dist/rpmbuild/SOURCES
	cp dist/kmd-$(VERSION_RPM).tar.gz dist/rpmbuild/SOURCES/
	rpmbuild --define "_topdir $(CURDIR)/dist/rpmbuild" $(if $(filter srpm,$@),-bs,-ba) packaging/kmd.spec
	-command -v rpmlint >/dev/null && rpmlint -r packaging/kmd.rpmlintrc packaging/kmd.spec dist/rpmbuild/SRPMS/*.rpm dist/rpmbuild/RPMS/*/*.rpm

## Install ------------------------------------------------------------------
install: build
	install -Dm0755 $(BIN) $(PREFIX)/bin/kmd
	install -Dm0644 docs/kmd.1 $(DATADIR)/man/man1/kmd.1
	$(BIN) --completion bash | install -Dm0644 /dev/stdin $(DATADIR)/bash-completion/completions/kmd
	$(BIN) --completion zsh  | install -Dm0644 /dev/stdin $(DATADIR)/zsh/site-functions/_kmd
	$(BIN) --completion fish | install -Dm0644 /dev/stdin $(DATADIR)/fish/vendor_completions.d/kmd.fish
	@echo "installed $(PREFIX)/bin/kmd (+ man page and completions)"

uninstall:
	rm -f $(PREFIX)/bin/kmd $(DATADIR)/man/man1/kmd.1 $(DATADIR)/bash-completion/completions/kmd \
	      $(DATADIR)/zsh/site-functions/_kmd $(DATADIR)/fish/vendor_completions.d/kmd.fish

## Tooling ------------------------------------------------------------------
tools: $(TOOLS)/staticcheck $(TOOLS)/govulncheck $(TOOLS)/gosec

$(TOOLS)/staticcheck:
	GOBIN=$(TOOLS) $(GO) install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

$(TOOLS)/govulncheck:
	GOBIN=$(TOOLS) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(TOOLS)/gosec:
	GOBIN=$(TOOLS) $(GO) install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)

hooks:
	git config core.hooksPath .githooks
	@echo "pre-push hook enabled: 'make ci' runs before every push"

clean:
	rm -rf bin dist coverage.out
