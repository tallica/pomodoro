APP         := Pomodoro
EXEC        := pomodoro
BUNDLE      := $(APP).app
IDENTIFIER  := pl.tallica.pomodoro
VERSION     := 0.1.0

# Ad-hoc signing by default. macOS only delivers notifications from a signed
# bundle, so even a local build has to be signed. Override with a Developer ID
# to ship it elsewhere: make bundle IDENTITY="Developer ID Application: ..."
IDENTITY    ?= -
ICON_COLOR  ?= \#D64541
INSTALL_DIR ?= /Applications

BINARY   := $(BUNDLE)/Contents/MacOS/$(EXEC)
PLIST    := $(BUNDLE)/Contents/Info.plist
ICON     := $(BUNDLE)/Contents/Resources/icon.icns
SOURCES  := $(shell find . -name '*.go' -not -path './$(BUNDLE)/*')

.PHONY: all
all: bundle

.PHONY: bundle
bundle: $(BINARY) $(PLIST) $(ICON) ## Build and sign Pomodoro.app
	@codesign -f -s "$(IDENTITY)" $(if $(filter-out -,$(IDENTITY)),--options runtime --timestamp,) "$(BUNDLE)"
	@echo "Built $(BUNDLE)"

$(BINARY): $(SOURCES) go.mod go.sum
	@mkdir -p "$(dir $@)"
	go build -trimpath -ldflags "-s -w" -o "$@" .

$(ICON):
	@mkdir -p "$(dir $@)"
	go run github.com/caseymrm/menuet/v2/cmd/appicon -name "$(APP)" -color "$(ICON_COLOR)" -o "$@"

# LSUIElement keeps the app out of the Dock and the app switcher: it lives in
# the status bar only.
$(PLIST): Makefile
	@mkdir -p "$(dir $@)"
	@printf '%s\n' \
	  '<?xml version="1.0" encoding="UTF-8"?>' \
	  '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
	  '<plist version="1.0">' \
	  '<dict>' \
	  '  <key>CFBundleExecutable</key><string>$(EXEC)</string>' \
	  '  <key>CFBundleIdentifier</key><string>$(IDENTIFIER)</string>' \
	  '  <key>CFBundleName</key><string>$(APP)</string>' \
	  '  <key>CFBundleDisplayName</key><string>$(APP)</string>' \
	  '  <key>CFBundleIconFile</key><string>icon</string>' \
	  '  <key>CFBundlePackageType</key><string>APPL</string>' \
	  '  <key>CFBundleShortVersionString</key><string>$(VERSION)</string>' \
	  '  <key>CFBundleVersion</key><string>$(VERSION)</string>' \
	  '  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>' \
	  '  <key>LSMinimumSystemVersion</key><string>13.0</string>' \
	  '  <key>LSUIElement</key><true/>' \
	  '  <key>NSHighResolutionCapable</key><true/>' \
	  '  <key>NSSupportsAutomaticGraphicsSwitching</key><true/>' \
	  '</dict>' \
	  '</plist>' > "$@"

.PHONY: run
run: bundle ## Build, then launch the app
	@pkill -x $(EXEC) 2>/dev/null || true
	open "$(BUNDLE)"

.PHONY: install
install: bundle ## Copy the app into /Applications
	@pkill -x $(EXEC) 2>/dev/null || true
	rm -rf "$(INSTALL_DIR)/$(BUNDLE)"
	cp -R "$(BUNDLE)" "$(INSTALL_DIR)/"
	@echo "Installed to $(INSTALL_DIR)/$(BUNDLE)"

.PHONY: uninstall
uninstall: ## Remove the app (leaves your session history alone)
	@pkill -x $(EXEC) 2>/dev/null || true
	rm -rf "$(INSTALL_DIR)/$(BUNDLE)"

.PHONY: test
test: ## Run the unit tests
	go test ./...

.PHONY: check
check: ## Format check, vet and test
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...

.PHONY: preview
preview: $(BINARY) $(PLIST) ## Dump the menu as JSON without opening a window
	MENUET_SNAPSHOT_PATH=menu-preview.json MENUET_SNAPSHOT_DELAY=1s "./$(BINARY)"
	@echo "Wrote menu-preview.json"

.PHONY: clean
clean: ## Remove build output
	rm -rf "$(BUNDLE)" bin menu-preview.json

.PHONY: help
help: ## List targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[1m%-12s\033[0m %s\n", $$1, $$2}'
