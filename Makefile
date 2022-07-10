DOCKER ?= podman

clean:
	rm -f ./metrics
build:
	env CGO_ENABLED=0 go build -a -o metrics ./cmd/metrics/main.go
dev:
	IP_ADDR=192.168.6.3:192.168.6.2 go run ./cmd/metrics/main.go
download:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.42.0
lint:
	./bin/golangci-lint run --fix
docker-build:
	$(DOCKER) build --rm -t ghcr.io/rich7690/smartplugs:latest . && $(DOCKER) push ghcr.io/rich7690/smartplugs:latest
