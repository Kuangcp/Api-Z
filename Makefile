GO ?= go
export CGO_ENABLED=0

.PHONY: build run test vet fmt migrate icon clean

build:
	$(GO) build -trimpath -o api-z .

run:
	$(GO) run .

test:
	$(GO) test -p 1 ./...

vet:
	$(GO) vet ./...
	$(GO) tool mygo vet .

fmt:
	gofmt -w .

migrate:
	$(GO) run . -migrate

icon:
	./tools/genicon.sh

clean:
	rm -f api-z
