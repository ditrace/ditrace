VERSION := $(shell git describe --always --tags --abbrev=0 | tail -c +2)
RELEASE := $(shell git describe --always --tags | awk -F- '{ if ($$2) dot="."} END { printf "1%s%s%s%s\n",dot,$$2,dot,$$3}')
VENDOR := "SKB Kontur"

default: prepare test build

build:
	mkdir -p build/root/usr/bin/
	go build -ldflags "-X main.version=${VERSION}-${RELEASE}" -o build/root/usr/bin/ditrace

build_windows:
	mkdir -p build
	go build -ldflags "-X main.version=${VERSION}-${RELEASE}" -o build/ditrace.exe

test_travis: clean prepare prepare_test
	golint ./...
	cd tests
	ginkgo -r --randomizeAllSpecs -cover --failOnPending -coverpkg=../ditrace --trace --race
	gover
	goveralls -coverprofile=gover.coverprofile -service=travis-ci

test: clean prepare prepare_test
	ginkgo --randomizeAllSpecs --randomizeSuites --failOnPending --trace --progress tests

prepare_test:
	go get github.com/axw/gocov/gocov
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover
	go get github.com/modocache/gover
	go get github.com/golang/lint/golint
	go get github.com/onsi/ginkgo/ginkgo

prepare:
	go get github.com/kardianos/govendor
	govendor sync

rpm: build
	fpm -t rpm \
		-s "dir" \
		--description "Distributed system gate" \
		-C ./build/root/ \
		--vendor ${VENDOR} \
		--url "https://github.com/ditrace/ditrace" \
		--name "ditrace" \
		--version "${VERSION}" \
		--iteration "${RELEASE}" \
		-p build

clean:
	rm -rf build

.PHONY: test
