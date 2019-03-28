package envoy

import (
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"k8s.io/api/extensions/v1beta1"
	"time"
)

func (e *EnvoyCluster) makeEnvoyClusters(envoyClustersChan chan []cache.Resource) {
	envoyClusters := []cache.Resource{}
	clusterMap := make(map[string]string)

	// Create makeEnvoyCluster Clusters per K8s Service referenced in ingress
	for _, k8sCluster := range e.K8sCacheStoreMap {
		for _, obj := range k8sCluster.IngressCacheStore.List() {
			ingress := obj.(*v1beta1.Ingress)
			for _, ingressRule := range ingress.Spec.Rules {
				for _, httpPath := range ingressRule.HTTP.Paths {
					clusterName := getClusterName(ingress.Namespace, ingressRule.Host, httpPath.Backend.ServiceName, httpPath.Backend.ServicePort.IntVal)
					clusterMap[clusterName] = clusterName
				}
			}
		}
	}

	for _, cluster := range clusterMap {
		refreshDelay := time.Second * 30
		envoyCluster := e.makeEnvoyCluster(cluster, refreshDelay, e.makeGrpcServices())
		envoyClusters = append(envoyClusters, &envoyCluster)
	}

	envoyClustersChan <- envoyClusters
}

func (e *EnvoyCluster) makeGrpcServices() []*core.GrpcService {
	grpcServices := []*core.GrpcService{}
	grpcService := &core.GrpcService{
		TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
				ClusterName: "xds_cluster",
			},
		},
	}
	grpcServices = append(grpcServices, grpcService)
	return grpcServices
}

func (e *EnvoyCluster) makeEnvoyCluster(cluster string, refreshDelay time.Duration, grpcServices []*core.GrpcService) v2.Cluster {
	return v2.Cluster{
		Name:           cluster,
		ConnectTimeout: time.Second * 5,
		LbPolicy:       v2.Cluster_ROUND_ROBIN,
		Type:           v2.Cluster_EDS,
		EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
			EdsConfig: &core.ConfigSource{
				ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
					ApiConfigSource: &core.ApiConfigSource{
						ApiType:      core.ApiConfigSource_GRPC,
						RefreshDelay: &refreshDelay,
						GrpcServices: grpcServices,
					},
				},
			},
		},
	}
}