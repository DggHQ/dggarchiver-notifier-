.PHONY: build ko lint

GOFLAGS := -tags netgo

DEBUG ?= 1
ifeq ($(DEBUG), 1)
	LDFLAGS := '-extldflags="-static"'
else
	GOFLAGS += -trimpath
	LDFLAGS := '-s -w -extldflags="-static"'
endif

GOFLAGS += -ldflags ${LDFLAGS}

build:
	CGO_ENABLED=0 go build ${GOFLAGS} -v -o target/dggarchiver-notifier

ko:
	ko build --local --base-import-paths

lint:
	golangci-lint run