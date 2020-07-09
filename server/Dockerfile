FROM alpine:latest

RUN mkdir -p /home/tiupserver/mirror/upstream

WORKDIR /home/tiupserver

COPY ./bin ./bin

EXPOSE 8989

ENTRYPOINT [ "./bin/tiup-server", "./mirror",\
    "--addr", "0.0.0.0:8989",\ 
    "--index", "./keys/index.json",\
    "--snapshot", "./keys/snapshot.json", \
    "--timestamp", "./keys/timestamp.json",\
    "--tiuphome", "./mirror/upstream",\
    "--owner", "./mirror/private/private.json",\
    "--ownerpub", "./mirror/private/public.json"]