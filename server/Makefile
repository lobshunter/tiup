build:
	go build -ldflags "-linkmode external -extldflags -static" -o ./bin/tiup-server .
	# this server used to use inotify, so it's built with static link to run in docker 
	# maybe it's okay to use command below to build now
	# CGO_ENABLE=0 go build -o ./bin/tiup-server .

docker:
	docker build --tag=hub.pingcap.net/py/tiupserver:merge .

docker-push:
	docker push hub.pingcap.net/py/tiupserver:merge