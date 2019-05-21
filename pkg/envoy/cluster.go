package envoy

import (
	"strconv"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	v2cluster "github.com/envoyproxy/go-control-plane/envoy/api/v2/cluster"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/gogo/protobuf/types"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/extensions/v1beta1"
)

func (e *EnvoyCluster) makeEnvoyClusters(envoyClustersChan chan []cache.Resource) {
	envoyClusters := []cache.Resource{}
	clusterMap := make(map[string]*v1beta1.Ingress)

	// Create makeEnvoyCluster Clusters per K8s Service referenced in ingress
	for _, k8sCluster := range e.K8sCacheStoreMap {
		for _, obj := range k8sCluster.IngressCacheStore.List() {
			ingress := obj.(*v1beta1.Ingress)
			for _, ingressRule := range ingress.Spec.Rules {
				for _, httpPath := range ingressRule.HTTP.Paths {
					clusterName := getClusterName(ingress.Namespace, ingressRule.Host, httpPath.Backend.ServiceName, httpPath.Backend.ServicePort.IntVal)
					clusterMap[clusterName] = ingress
				}
			}
		}
	}

	for cluster, ing := range clusterMap {
		refreshDelay := time.Second * 30
		envoyCluster := e.makeEnvoyCluster(cluster, ing, refreshDelay, e.makeGrpcServices())
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

func (e *EnvoyCluster) makeEnvoyCluster(cluster string, ing *v1beta1.Ingress, refreshDelay time.Duration, grpcServices []*core.GrpcService) v2.Cluster {
	healthChecks := []*core.HealthCheck{}
	timeout := 10 * time.Second
	interval := 15 * time.Second
	healthCheck := &core.HealthCheck{
		Timeout:            &timeout,
		Interval:           &interval,
		UnhealthyThreshold: &types.UInt32Value{Value: 2},
		HealthyThreshold:   &types.UInt32Value{Value: 2},
		HealthChecker: &core.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &core.HealthCheck_TcpHealthCheck{},
		},
	}
	healthChecks = append(healthChecks, healthCheck)

	maxConnections, err := strconv.ParseUint(getAnnotation(ing, ANNOTATIONS_THRESHOLDS_CIRCUITBREAKERS_CLUSTER_ENVOYPROXY_MAXCONNECTIONS, "1024"), 10, 32)
	if err != nil {
		log.WithError(err).Fatal("failed to parse maxConnections annotation")
	}
	maxPendingRequests, err := strconv.ParseUint(getAnnotation(ing, ANNOTATIONS_THRESHOLDS_CIRCUITBREAKERS_CLUSTER_ENVOYPROXY_MAXPENDINGREQUESTS, "1024"), 10, 32)
	if err != nil {
		log.WithError(err).Fatal("failed to parse maxPendingRequests annotation")
	}
	maxRequests, err := strconv.ParseUint(getAnnotation(ing, ANNOTATIONS_THRESHOLDS_CIRCUITBREAKERS_CLUSTER_ENVOYPROXY_MAXREQUESTS, "1024"), 10, 32)
	if err != nil {
		log.WithError(err).Fatal("failed to parse maxRequests annotation")
	}
	maxRetries, err := strconv.ParseUint(getAnnotation(ing, ANNOTATIONS_THRESHOLDS_CIRCUITBREAKERS_CLUSTER_ENVOYPROXY_MAXRETRIES, "3"), 10, 32)
	if err != nil {
		log.WithError(err).Fatal("failed to parse maxRetries annotation")
	}

	thresholds := []*v2cluster.CircuitBreakers_Thresholds{}
	threshold := &v2cluster.CircuitBreakers_Thresholds{
		Priority:           core.RoutingPriority_DEFAULT,
		MaxConnections:     &types.UInt32Value{Value: uint32(maxConnections)},
		MaxPendingRequests: &types.UInt32Value{Value: uint32(maxPendingRequests)},
		MaxRequests:        &types.UInt32Value{Value: uint32(maxRequests)},
		MaxRetries:         &types.UInt32Value{Value: uint32(maxRetries)},
	}
	thresholds = append(thresholds, threshold)
	circuitBreakers := &v2cluster.CircuitBreakers{
		Thresholds: thresholds,
	}

	return v2.Cluster{
		Name:                          cluster,
		ConnectTimeout:                time.Second * 5,
		PerConnectionBufferLimitBytes: &types.UInt32Value{Value: 1024 * 1024 * 100},
		CircuitBreakers:               circuitBreakers,
		LbPolicy:                      v2.Cluster_ROUND_ROBIN,
		ClusterDiscoveryType:          &v2.Cluster_Type{Type: v2.Cluster_EDS},
		HealthChecks:                  healthChecks,
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
