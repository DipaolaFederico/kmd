# kmd — local CI. `make ci` runs every gate; any failure stops the pipeline.

GO          ?= go
BIN         := bin/kmd
BIN_AI      := bin/kmd-ai
PREFIX      ?= $(HOME)/.local
VERSION     ?= $(shell scripts/build-version.sh 2>/dev/null || echo dev)
LDFLAGS      = -s -w -X github.com/DipaolaFederico/kmd/internal/cli.Version=$(VERSION)

# Build tags for the tag-sensitive gates (make test-ai sets kmdai). Passed
# as -tags: GOFLAGS is ignored by deadcode -test and by gosec.
GOTAGS      ?=
TAGS        := -tags '$(GOTAGS)'

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
GOCOGNIT_VERSION    := v1.3.0
DUPL_VERSION        := v1.1.0
DEADCODE_VERSION    := v0.53.0

# Lint limits (see CONTRIBUTING.md, "Style").
COGNIT_LIMIT        := 30
DUPL_TOKENS         := 100

VERSION_RPM := $(shell sed -n 's/^Version: *//p' packaging/kmd.spec)
APPID       := io.github.dipaolafederico.kmd
DATADIR     ?= $(PREFIX)/share

.PHONY: ci fmt fmt-check lint vet staticcheck complexity dupl deadcode scripts-test test spec cover api api-update security vuln gosec \
        build perf build-ai test-ai fuzz bench install uninstall tools hooks clean packaging dist srpm rpm docs-images

ci: fmt-check lint scripts-test test cover security build packaging perf
	@printf '\n\033[1;32m✔ local CI passed\033[0m\n'

## Formatting ---------------------------------------------------------------
fmt:
	gofmt -s -w .

fmt-check:
	@echo '» fmt'
	@out=$$(gofmt -s -l .); if [ -n "$$out" ]; then echo "not gofmt'ed:"; echo "$$out"; exit 1; fi

## Lint ---------------------------------------------------------------------
lint: vet staticcheck complexity dupl deadcode

vet:
	@echo '» go vet'
	@$(GO) vet $(TAGS) ./...

staticcheck: $(TOOLS)/staticcheck
	@echo '» staticcheck'
	@$(TOOLS)/staticcheck $(TAGS) ./...

# Cognitive complexity within the limit for functions a change adds or makes
# more complex (a ratchet: complex code that exists is not flagged until
# touched). STRICT=1 (CI) fails when there is no main branch to compare with.
complexity: $(TOOLS)/gocognit
	@echo '» complexity (gocognit)'
	@STRICT=$(STRICT) ./scripts/complexity.sh $(TOOLS)/gocognit $(COGNIT_LIMIT)

# No copy-pasted blocks of DUPL_TOKENS tokens or more, tests included.
dupl: $(TOOLS)/dupl
	@echo '» duplication (dupl)'
	@out=$$(git ls-files '*.go' | $(TOOLS)/dupl -files -t $(DUPL_TOKENS) -plumbing 2>&1) || exit 1; \
	if [ -n "$$out" ]; then echo "$$out"; echo "factor out the duplicated code"; exit 1; fi

# No function unreachable from main or the tests (-test: code used only by
# tests, such as the public render package's API, counts as reachable).
deadcode: $(TOOLS)/deadcode
	@echo '» dead code (deadcode)'
	@out=$$($(TOOLS)/deadcode $(TAGS) -test ./...) || exit 1; \
	if [ -n "$$out" ]; then echo "$$out"; echo "remove the unused code"; exit 1; fi

## Tests --------------------------------------------------------------------
scripts-test:
	@echo '» scripts'
	@./scripts/test-scripts.sh

test:
	@echo '» test (race detector)'
	@$(GO) test $(TAGS) -race -count=1 ./...

spec:
	@echo '» CommonMark 0.31.2 + GFM spec conformance'
	@$(GO) test -count=1 -run 'Spec' -v ./internal/md/ | grep -E 'examples pass'

cover:
	@echo '» coverage (min $(COVER_MIN)%)'
	@# quiet on success; on failure show the failing tests (not the ok lines)
	@out=$$($(GO) test $(TAGS) -count=1 -covermode=atomic -coverprofile=coverage.out ./internal/... ./render/... 2>&1) || \
		{ printf '%s\n' "$$out" | grep -vE '^(ok|\?) '; exit 1; }
	@$(GO) tool cover -func=coverage.out | awk -v min=$(COVER_MIN) '/^total:/ { \
		sub("%", "", $$3); printf "  total coverage: %s%%\n", $$3; \
		if ($$3 + 0 < min) { print "  coverage below " min "%"; exit 1 } }'

# The public package's exported API, checked against render/testdata/api.txt
# (also part of `make test`). api-update accepts additions; never use it to
# hide a removal or change after v1.0.0: that needs a new major version.
api:
	@echo '» public API (render/testdata/api.txt)'
	@$(GO) test -count=1 -run '^TestAPI$$' ./render/

api-update:
	$(GO) test -count=1 -run '^TestAPI$$' ./render/ -update-api

fuzz:
	$(GO) test -run='^$$' -fuzz=FuzzParse -fuzztime=$(FUZZ_TIME) ./internal/md/
	$(GO) test -run='^$$' -fuzz=FuzzRender -fuzztime=$(FUZZ_TIME) ./internal/render/
	$(GO) test -run='^$$' -fuzz=FuzzHighlight -fuzztime=$(FUZZ_TIME) ./internal/render/
	$(GO) test -run='^$$' -fuzz=FuzzMermaid -fuzztime=$(FUZZ_TIME) ./internal/diagram/
	$(GO) test -run='^$$' -fuzz=FuzzDisplay -fuzztime=$(FUZZ_TIME) ./internal/tex/
	$(GO) test -run='^$$' -fuzz=FuzzDataURI -fuzztime=$(FUZZ_TIME) ./internal/termimg/
	$(GO) test -run='^$$' -fuzz=FuzzBackground -fuzztime=$(FUZZ_TIME) ./internal/term/
	$(GO) test -tags kmdai -run='^$$' -fuzz=FuzzResponse -fuzztime=$(FUZZ_TIME) ./internal/ai/

bench:
	$(GO) test -run='^$$' -bench=. -benchmem ./internal/render/

## Security -----------------------------------------------------------------
security: vuln gosec

vuln: $(TOOLS)/govulncheck
	@echo '» govulncheck'
	@$(TOOLS)/govulncheck $(TAGS) ./...

gosec: $(TOOLS)/gosec
	@echo '» gosec'
	@$(TOOLS)/gosec $(TAGS) -quiet -fmt=text ./...

## Build & performance ------------------------------------------------------
build:
	@echo '» build'
	@CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) .
	@ls -lh $(BIN) | awk '{ print "  binary size: " $$5 }'

perf: build
	@echo '» perf (end-to-end, < $(PERF_MAX_MS) ms)'
	@./scripts/perf.sh $(BIN) testdata/sample.md $(PERF_MAX_MS) $(PERF_RUNS)

## Experimental AI (docs/experimental/ai.md) ------------------------------
# Only builds made with -tags kmdai contain it; the released binary does not.
build-ai:
	@echo '» build (-tags kmdai)'
	@# +kmdai in --version: this binary may contact a local server, unlike a release
	@CGO_ENABLED=0 $(GO) build -trimpath -tags kmdai -ldflags '$(LDFLAGS)+kmdai' -o $(BIN_AI) .
	@ls -lh $(BIN_AI) | awk '{ print "  binary size: " $$5 }'

# the gates that depend on build tags, run on the tagged build
test-ai:
	@echo '» experimental AI (-tags kmdai)'
	@$(MAKE) --no-print-directory vet staticcheck deadcode test cover security GOTAGS=kmdai

## Packaging -----------------------------------------------------------------
# STRICT=1 (CI) fails when a package validator (ruby, dpkg-deb, lintian,
# python3-yaml) is missing instead of skipping it.
packaging: build
	@echo '» packaging metadata (man page, AppStream, completions, spec version, packages)'
	@groff -man -Tutf8 -ww -z docs/kmd.1
	@appstreamcli validate --no-net --pedantic packaging/$(APPID).metainfo.xml >/dev/null
	@for sh in bash zsh fish; do $(BIN) --completion $$sh >/dev/null || exit 1; done
	@grep -q '"kmd $(VERSION_RPM)"' docs/kmd.1 || { echo "  docs/kmd.1 version != spec $(VERSION_RPM)"; exit 1; }
	@grep -q 'version="$(VERSION_RPM)"' packaging/$(APPID).metainfo.xml || { echo "  metainfo lacks release $(VERSION_RPM)"; exit 1; }
	@STRICT=$(STRICT) ./scripts/check-packages.sh $(BIN)

# Source tarball with the layout of a GitHub release archive (kmd-X.Y.Z/).
# Uses the files git tracks or would track (works before the first commit),
# except those .gitattributes marks export-ignore, as `git archive` does.
dist:
	@mkdir -p dist
	git ls-files -z --cached --others --exclude-standard -- . ':(exclude,attr:export-ignore)' | \
	  tar --null -T - --transform 's,^,kmd-$(VERSION_RPM)/,' --sort=name --owner=0 --group=0 \
	      --mtime='2026-01-01' -czf dist/kmd-$(VERSION_RPM).tar.gz
	@echo "dist/kmd-$(VERSION_RPM).tar.gz"

# Local RPM build (needs: sudo dnf install rpm-build go-rpm-macros libappstream-glib rpmlint).
srpm rpm: dist
	@mkdir -p dist/rpmbuild/SOURCES
	cp dist/kmd-$(VERSION_RPM).tar.gz dist/rpmbuild/SOURCES/
	rpmbuild --define "_topdir $(CURDIR)/dist/rpmbuild" $(if $(filter srpm,$@),-bs,-ba) packaging/kmd.spec
	-command -v rpmlint >/dev/null && rpmlint -r packaging/kmd.rpmlintrc packaging/kmd.spec dist/rpmbuild/SRPMS/*.rpm dist/rpmbuild/RPMS/*/*.rpm

## README images (CONTRIBUTING.md, "README images") ------------------------
# Needs documentation-only tools (listed there), never build dependencies.
# TAPES=docs/images/tapes/viewer.tape regenerates only that image.
docs-images: build
	@./scripts/docs-images.sh $(BIN) $(TAPES)

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
tools: $(TOOLS)/staticcheck $(TOOLS)/govulncheck $(TOOLS)/gosec $(TOOLS)/gocognit $(TOOLS)/dupl $(TOOLS)/deadcode

$(TOOLS)/staticcheck:
	GOBIN=$(TOOLS) $(GO) install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

$(TOOLS)/govulncheck:
	GOBIN=$(TOOLS) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(TOOLS)/gosec:
	GOBIN=$(TOOLS) $(GO) install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)

$(TOOLS)/gocognit:
	GOBIN=$(TOOLS) $(GO) install github.com/uudashr/gocognit/cmd/gocognit@$(GOCOGNIT_VERSION)

$(TOOLS)/dupl:
	GOBIN=$(TOOLS) $(GO) install github.com/mibk/dupl@$(DUPL_VERSION)

$(TOOLS)/deadcode:
	GOBIN=$(TOOLS) $(GO) install golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)

hooks:
	git config core.hooksPath .githooks
	@echo "pre-push hook enabled: 'make ci' runs before every push"

clean:
	rm -rf bin dist coverage.out
