#!/usr/bin/env bash
docker build test/ -t docker.target.com/kubernetes/envoyproxy-test
docker run -p 9901:9901 -p 80:80 -p 443:443 docker.target.com/kubernetes/envoyproxy-test --max-obj-name-len 300 --config-path /etc/envoy/envoy.yaml
