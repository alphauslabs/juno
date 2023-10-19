FROM golang:1.21.3-bookworm
COPY . /go/src/github.com/alphauslabs/juno/
WORKDIR /go/src/github.com/alphauslabs/juno/
RUN CGO_ENABLED=0 GOOS=linux go build -v -trimpath -installsuffix cgo -o juno .

FROM debian:stable-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /juno/
COPY --from=0 /go/src/github.com/alphauslabs/juno/juno .
ENTRYPOINT ["/juno/juno"]
CMD ["-logtostderr"]
