VERSION := $(shell sh -c 'git describe --always --tags')

default: prepare test build

build:
	mkdir -p build/root/usr/bin/ 
	go build -ldflags "-X main.version=$(VERSION)" -o build/root/usr/bin/ditrace

test: clean prepare
	go get github.com/onsi/ginkgo/ginkgo
	ginkgo --randomizeAllSpecs --randomizeSuites --failOnPending --trace --progress tests

prepare:
	go get github.com/sparrc/gdm
	gdm restore

rpm: build
	fpm -t rpm \
		-s "dir" \
		--description "Distributed system gate" \
		-C ./build/root/ \
		--vendor "SKB Kontur" \
		--url "https://github.com/ditrace/ditrace" \
		--name "ditrace" \
		--version "$(VERSION)" \
		-p build

clean:
	rm -rf build

.PHONY: test
