package envoy

import (
	"git.target.com/Kubernetes/envoy-control-plane/pkg/data"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
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
						ingressHost: ingressRule.Host,
						port:        httpPath.Backend.ServicePort.IntVal,
					}
					clusterMap[clusterName] = k8sService
				}
			}
		}
	}

	for _, k8sCluster := range e.K8sCacheStoreMap {
		for clusterName, k8sService := range clusterMap {
			serviceObj, serviceExists, err := k8sCluster.ServiceCacheStore.GetByKey(k8sService.namespace + "/" + k8sService.name)
			if err != nil {
				log.Fatalf("Error in getting the service %v-%v by name", k8sService.namespace, k8sService.name)
			}
			ingressObj, ingressExists, err := k8sCluster.IngressCacheStore.GetByKey(k8sService.namespace + "/" + k8sService.ingressName)
			if err != nil {
				log.Fatalf("Error in getting the ingress %v-%v by name", k8sService.namespace, k8sService.ingressName)
			}
			if serviceExists && ingressExists {
				ingress := ingressObj.(*v1beta1.Ingress)
				service := serviceObj.(*v1.Service)
				if service.Spec.Type == v1.ServiceTypeNodePort && isIngressForEnvoyCluster(ingress, k8sService.ingressHost) {
					lbEndpoints := []endpoint.LbEndpoint{}
					localityLbEndpoint := endpoint.LocalityLbEndpoints{}
					for _, servicePort := range service.Spec.Ports {
						for _, nodeObj := range k8sCluster.NodeCacheStore.List() {
							node := nodeObj.(*v1.Node)
							lbEndpoint := e.makeEnvoyLbEndpoint(node, servicePort)
							lbEndpoints = append(lbEndpoints, lbEndpoint)
							//TODO: Debuugging, add only one node as the endpoint.
							break
						}
					}
					localityLbEndpoint = e.makeEnvoyLocalityLbEndpoint(k8sCluster, lbEndpoints)
					localityLbEndpointsMap[clusterName] = append(localityLbEndpointsMap[clusterName], localityLbEndpoint)
				}
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
			Zone: k8sCluster.Zone,
		},
		Priority:    k8sCluster.Priority,
		LbEndpoints: lbEndpoints,
	}
}

func isIngressForEnvoyCluster(ingress *v1beta1.Ingress, host string) bool {
	for _, rule := range ingress.Spec.Rules {
		if rule.Host == host {
			return true
		}
	}
	return false
}
