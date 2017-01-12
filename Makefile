VERSION := $(shell git describe --always --tags --abbrev=0 | tail -c +2)
RELEASE := $(shell git describe --always --tags | awk -F- '{ if ($$2) dot="."} END { printf "1%s%s%s%s\n",dot,$$2,dot,$$3}')
VENDOR := "SKB Kontur"

default: prepare test build

build:
	mkdir -p build/root/usr/bin/
	go build -ldflags "-X main.version=$(VERSION)-$(RELEASE)" -o build/root/usr/bin/ditrace

build_windows:
	mkdir -p build
	go build -ldflags "-X main.version=$(VERSION)-$(RELEASE)" -o build/ditrace.exe

test: clean prepare
	go get github.com/onsi/ginkgo/ginkgo
	ginkgo --randomizeAllSpecs --randomizeSuites --failOnPending --trace --progress tests

prepare:
	go get github.com/kardianos/govendor
	govendor sync

rpm: build
	fpm -t rpm \
		-s "dir" \
		--description "Distributed system gate" \
		-C ./build/root/ \
		--vendor $(VENDOR) \
		--url "https://github.com/ditrace/ditrace" \
		--name "ditrace" \
		--version "$(VERSION)" \
		--iteration "$(RELEASE)" \
		-p build

clean:
	rm -rf build

.PHONY: test
