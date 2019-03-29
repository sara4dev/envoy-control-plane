# envoy-control-plane
[![Build Status](https://drone6.target.com/api/badges/Kubernetes/envoy-control-plane/status.svg)](https://drone6.target.com/Kubernetes/envoy-control-plane)

control plane for envoy

## Creating a multi-cluster ingress


For each Service you are planning to use in the multi-cluster ingress, it must be configured the same across all of the clusters.

* Have the same ingress host name in all of the clusters.
* Have the same service name in all of the clusters.
* Be in the same namespace in all of the clusters.
* Be of type NodePort.

