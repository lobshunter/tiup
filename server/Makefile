all: build docker docker-push

build:
	go build -ldflags "-linkmode external -extldflags -static" -o ./bin/tiup-server .

docker:
	docker build --tag=hub.pingcap.net/py/tiupserver:merge .

docker-push:
	docker push hub.pingcap.net/py/tiupserver:merge