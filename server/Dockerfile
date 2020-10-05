FROM alpine:latest

RUN mkdir -p /home/tiupserver/mirror \
    mkdir -p /home/tiupserver/mirror/private \
    mkdir -p /home/tiupserver/keys \
    mkdir -p /home/tiupserver/upstream/bin \
    mkdir -p /home/tiupserver/upstream/manifests

ENV TIUP_VERBOSE=enable
WORKDIR /home/tiupserver

# COPY tools/root.json upstream/bin/root.json
# COPY tools/pri.index.json keys/index.json
# COPY tools/pri.snapshot.json keys/snapshot.json
# COPY tools/pri.timestamp.json keys/timestamp.json
# COPY tools/private.json mirror/private/private.json
# COPY tools/public.json mirror/private/public.json
COPY ./bin ./bin

EXPOSE 8989

# ENTRYPOINT [ "bin/tiup-server.sh", "./mirror",\
ENTRYPOINT [ "./bin/tiup-server", "./mirror",\
    "--addr", "0.0.0.0:8989",\ 
    "--index", "./keys/index.json",\
    "--snapshot", "./keys/snapshot.json", \
    "--timestamp", "./keys/timestamp.json",\
    "--tiuphome", "./upstream",\
    "--owner", "./mirror/private/private.json",\
    "--ownerpub", "./mirror/private/public.json"]