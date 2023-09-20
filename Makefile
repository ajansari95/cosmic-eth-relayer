.PHONY: build

version ?= latest

build:
	go build

docker:
	docker build -f Dockerfile . -t ajansari95/cosmic-relayer:${version}

docker-local:
	go build
	docker build -f Dockerfile.local . -t ajansari95/cosmic-relayer:${version}

docker-push:
	docker push ajansari95/cosmic-relayer:${version}
