FROM golang:1.12.0-alpine as builder
WORKDIR /go/src/git.target.com/Kubernetes/envoy-control-plane
COPY . .
RUN apk add --no-cache git && \
    go get -d ./... && \
    go build

FROM alpine as runner
ENTRYPOINT [ "/envoy-control-plane" ]
COPY --from=builder /go/src/git.target.com/Kubernetes/envoy-control-plane/envoy-control-plane envoy-control-plane