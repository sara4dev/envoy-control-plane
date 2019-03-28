package envoy

import (
	"git.target.com/Kubernetes/envoy-control-plane/pkg/data"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"log"
	"strconv"
)

func (e *EnvoyCluster) makeEnvoyEndpoints(envoyEndpointsChan chan []cache.Resource) {
	envoyEndpoints := []cache.Resource{}
	localityLbEndpointsMap := make(map[string][]endpoint.LocalityLbEndpoints)

	clusterMap := make(map[string]k8sService)
	// Create makeEnvoyCluster Clusters per K8s Service referenced in ingress

	for _, k8sCluster := range e.K8sCacheStoreMap {
		for _, obj := range k8sCluster.IngressCacheStore.List() {
			ingress := obj.(*v1beta1.Ingress)
			for _, ingressRule := range ingress.Spec.Rules {
				for _, httpPath := range ingressRule.HTTP.Paths {
					//TODO consider services with StrVal
					clusterName := getClusterName(ingress.Namespace, ingressRule.Host, httpPath.Backend.ServiceName, httpPath.Backend.ServicePort.IntVal)
					k8sService := k8sService{
						name:        httpPath.Backend.ServiceName,
						ingressName: ingress.Name,
						namespace:   ingress.Namespace,
						port:        httpPath.Backend.ServicePort.IntVal,
					}
					clusterMap[clusterName] = k8sService
				}
			}
		}
	}

	for _, k8sCluster := range e.K8sCacheStoreMap {
		for clusterName, k8sService := range clusterMap {
			lbEndpoints := []endpoint.LbEndpoint{}
			localityLbEndpoint := endpoint.LocalityLbEndpoints{}
			serviceObj, serviceExists, err := k8sCluster.ServiceCacheStore.GetByKey(k8sService.namespace + "/" + k8sService.name)
			if err != nil {
				log.Fatal("Error in getting service by name")
			}
			_, ingressExists, err := k8sCluster.IngressCacheStore.GetByKey(k8sService.namespace + "/" + k8sService.ingressName)
			if err != nil {
				log.Fatal("Error in getting ingress by name")
			}
			if serviceExists && ingressExists {
				service := serviceObj.(*v1.Service)
				if service.Spec.Type == v1.ServiceTypeNodePort {
					for _, servicePort := range service.Spec.Ports {
						for _, nodeObj := range k8sCluster.NodeCacheStore.List() {
							node := nodeObj.(*v1.Node)
							lbEndpoint := e.makeEnvoyLbEndpoint(node, servicePort)
							lbEndpoints = append(lbEndpoints, lbEndpoint)
						}
					}
				}

				localityLbEndpoint = e.makeEnvoyLocalityLbEndpoint(k8sCluster, lbEndpoints)
				localityLbEndpointsMap[clusterName] = append(localityLbEndpointsMap[clusterName], localityLbEndpoint)
			}
		}
	}

	envoyEndpoints = e.makeEnvoyEndpoint(clusterMap, localityLbEndpointsMap, envoyEndpoints)

	envoyEndpointsChan <- envoyEndpoints
}

func (e *EnvoyCluster) makeEnvoyEndpoint(clusterMap map[string]k8sService, localityLbEndpointsMap map[string][]endpoint.LocalityLbEndpoints, envoyEndpoints []cache.Resource) []cache.Resource {
	// Each cluster needs an endpoint, even if its empty
	for clusterName, _ := range clusterMap {
		envoyEndpoint := v2.ClusterLoadAssignment{
			ClusterName: clusterName,
		}
		if localityLbEndpointsMap[clusterName] != nil {
			envoyEndpoint.Endpoints = localityLbEndpointsMap[clusterName]
		}
		envoyEndpoints = append(envoyEndpoints, &envoyEndpoint)
	}
	return envoyEndpoints
}

func (e *EnvoyCluster) makeEnvoyLbEndpoint(node *v1.Node, servicePort v1.ServicePort) endpoint.LbEndpoint {
	return endpoint.LbEndpoint{
		HostIdentifier: &endpoint.LbEndpoint_Endpoint{
			Endpoint: &endpoint.Endpoint{
				Address: &core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.TCP,
							// TODO fix the address
							Address: node.Status.Addresses[0].Address,
							PortSpecifier: &core.SocketAddress_PortValue{
								PortValue: uint32(servicePort.NodePort),
							},
						},
					},
				},
			},
		},
	}
}

func (e *EnvoyCluster) makeEnvoyLocalityLbEndpoint(k8sCluster *data.K8sCacheStore, lbEndpoints []endpoint.LbEndpoint) endpoint.LocalityLbEndpoints {
	return endpoint.LocalityLbEndpoints{
		Locality: &core.Locality{
			Zone: strconv.Itoa(int(k8sCluster.Zone)),
		},
		Priority:    k8sCluster.Priority,
		LbEndpoints: lbEndpoints,
	}
}