FROM golang:1.17.2-alpine3.13 AS servok-builder

ARG GRPC_HEALTH_PROBE_VERSION=0.3.6
RUN apk add curl
RUN curl -Lo /go/bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/v${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64
RUN chmod +x /go/bin/grpc_health_probe

WORKDIR /go/src/servok
RUN go env -w GOPRIVATE=github.com/authzed/servok

COPY ./go.mod ./go.sum .
RUN go mod download

COPY ./ /go/src/servok
RUN go build ./cmd/servok/

FROM alpine:3.14.2
RUN [ ! -e /etc/nsswitch.conf ] && echo 'hosts: files dns' > /etc/nsswitch.conf
COPY --from=servok-builder /go/bin/grpc_health_probe /usr/local/bin/
COPY --from=servok-builder /go/src/servok/servok /usr/local/bin/
CMD ["servok"]
