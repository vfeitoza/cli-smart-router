PLUGIN_ID := smart-model-router
PKG := ./cmd/plugin
DIST_DIR := dist

GO ?= go
CGO_ENABLED ?= 1

LINUX_AMD64_CC ?= x86_64-linux-gnu-gcc
LINUX_ARM64_CC ?= aarch64-linux-gnu-gcc
DARWIN_AMD64_CC ?= o64-clang
DARWIN_ARM64_CC ?= oa64-clang
WINDOWS_AMD64_CC ?= x86_64-w64-mingw32-gcc
WINDOWS_ARM64_CC ?= aarch64-w64-mingw32-gcc

.PHONY: all test build-local build-all build-linux build-darwin build-windows clean

all: test build-local

test:
	$(GO) test ./...

build-local:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -buildmode=c-shared -o $(DIST_DIR)/$(PLUGIN_ID).so $(PKG)

build-all: build-linux build-darwin build-windows

build-linux: $(DIST_DIR)/linux/amd64/$(PLUGIN_ID).so $(DIST_DIR)/linux/arm64/$(PLUGIN_ID).so

build-darwin: $(DIST_DIR)/darwin/amd64/$(PLUGIN_ID).dylib $(DIST_DIR)/darwin/arm64/$(PLUGIN_ID).dylib

build-windows: $(DIST_DIR)/windows/amd64/$(PLUGIN_ID).dll $(DIST_DIR)/windows/arm64/$(PLUGIN_ID).dll

$(DIST_DIR)/linux/amd64/$(PLUGIN_ID).so:
	mkdir -p $(dir $@)
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=$(LINUX_AMD64_CC) $(GO) build -buildmode=c-shared -o $@ $(PKG)

$(DIST_DIR)/linux/arm64/$(PLUGIN_ID).so:
	mkdir -p $(dir $@)
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=$(LINUX_ARM64_CC) $(GO) build -buildmode=c-shared -o $@ $(PKG)

$(DIST_DIR)/darwin/amd64/$(PLUGIN_ID).dylib:
	mkdir -p $(dir $@)
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC=$(DARWIN_AMD64_CC) $(GO) build -buildmode=c-shared -o $@ $(PKG)

$(DIST_DIR)/darwin/arm64/$(PLUGIN_ID).dylib:
	mkdir -p $(dir $@)
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC=$(DARWIN_ARM64_CC) $(GO) build -buildmode=c-shared -o $@ $(PKG)

$(DIST_DIR)/windows/amd64/$(PLUGIN_ID).dll:
	mkdir -p $(dir $@)
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=$(WINDOWS_AMD64_CC) $(GO) build -buildmode=c-shared -o $@ $(PKG)

$(DIST_DIR)/windows/arm64/$(PLUGIN_ID).dll:
	mkdir -p $(dir $@)
	CGO_ENABLED=1 GOOS=windows GOARCH=arm64 CC=$(WINDOWS_ARM64_CC) $(GO) build -buildmode=c-shared -o $@ $(PKG)

clean:
	rm -rf $(DIST_DIR)
	rm -f $(PLUGIN_ID).so $(PLUGIN_ID).h $(PLUGIN_ID).dylib $(PLUGIN_ID).dll
