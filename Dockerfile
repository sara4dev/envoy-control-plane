FROM alpine as runner
ENTRYPOINT [ "/envoy-control-plane" ]
COPY envoy-control-plane .
