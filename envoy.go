package main

import (
	"fmt"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const grpcMaxConcurrentStreams = 1000000

// Hasher returns node ID as an ID
type Hasher struct {
}

// ID function
func (h Hasher) ID(node *core.Node) string {
	if node == nil {
		return "unknown"
	}
	return node.Id
}

type logger struct{}

func (logger logger) Infof(format string, args ...interface{}) {
	log.Debugf(format, args...)
}
func (logger logger) Errorf(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

type callbacks struct {
	signal   chan struct{}
	fetches  int
	requests int
	mu       sync.Mutex
}

func (cb *callbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	log.WithFields(log.Fields{"fetches": cb.fetches, "requests": cb.requests}).Info("server callbacks")
}
func (cb *callbacks) OnStreamOpen(_ context.Context, id int64, typ string) error {
	log.Debugf("stream %d open for %s", id, typ)
	return nil
}
func (cb *callbacks) OnStreamClosed(id int64) {
	log.Debugf("stream %d closed", id)
}
func (cb *callbacks) OnStreamRequest(int64, *v2.DiscoveryRequest) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.requests++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}
	return nil
}
func (cb *callbacks) OnStreamResponse(int64, *v2.DiscoveryRequest, *v2.DiscoveryResponse) {}
func (cb *callbacks) OnFetchRequest(_ context.Context, req *v2.DiscoveryRequest) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.fetches++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}
	return nil
}
func (cb *callbacks) OnFetchResponse(*v2.DiscoveryRequest, *v2.DiscoveryResponse) {}

var (
	version            int32
	envoySnapshotCache envoycache.SnapshotCache
)

func RunManagementServer(ctx context.Context, server server.Server, port uint) {
	// gRPC golang library sets a very small upper bound for the number gRPC/h2
	// streams over a single TCP connection. If a proxy multiplexes requests over
	// a single connection to the management server, then it might lead to
	// availability problems.
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.WithError(err).Fatal("failed to listen")
	}

	// register services
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
	v2.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	v2.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	v2.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	v2.RegisterListenerDiscoveryServiceServer(grpcServer, server)
	discovery.RegisterSecretDiscoveryServiceServer(grpcServer, server)

	log.WithFields(log.Fields{"port": port}).Info("management server listening")
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Error(err)
		}
	}()
	<-ctx.Done()

	grpcServer.GracefulStop()
}

func createEnvoySnapshot() {
	atomic.AddInt32(&version, 1)
	//nodeId := envoySnapshotCache.GetStatusKeys()[0]

	log.Infof(">>>>>>>>>>>>>>>>>>> creating endpoints ")

	grpcServices := []*core.GrpcService{}
	grpcService := &core.GrpcService{
		TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
				ClusterName: "xds_cluster",
			},
		},
	}
	grpcServices = append(grpcServices, grpcService)

	envoyClusters := []envoycache.Resource{}
	envoyEndpoints := []envoycache.Resource{}
	envoyListeners := []envoycache.Resource{}
	//envoyRoutes := []envoycache.Resource{}

	virtualHosts := []route.VirtualHost{}

	virtualHostsMap := make(map[string]route.VirtualHost)

	for _, obj := range ingressK8sCacheStore.List() {
		ingress := obj.(*extbeta1.Ingress)

		for _, ingressRule := range ingress.Spec.Rules {
			virtualHost := makeVirtualHost(ingress.Namespace, ingressRule)
			existingVirtualHost := virtualHostsMap[virtualHost.Domains[0]]
			if existingVirtualHost.Name != "" {
				existingVirtualHost.Routes = append(existingVirtualHost.Routes, virtualHost.Routes...)
			} else {
				virtualHostsMap[virtualHost.Domains[0]] = virtualHost
			}
		}
	}

	for _, value := range virtualHostsMap {
		virtualHosts = append(virtualHosts, value)
	}

	httpConnectionManager := makeConnectionManager(virtualHosts)
	httpConfig, err := util.MessageToStruct(httpConnectionManager)
	if err != nil {
		log.Fatal("Error in converting connection manager")
	}

	envoyListener := &v2.Listener{
		Name: "http",
		Address: core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Address: "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: 80,
					},
				},
			},
		},
		//TODO fix the route part
		FilterChains: []listener.FilterChain{
			{
				Filters: []listener.Filter{
					{
						Name: util.HTTPConnectionManager,
						ConfigType: &listener.Filter_Config{
							Config: httpConfig,
						},
					},
				},
			},
		},
	}
	envoyListeners = append(envoyListeners, envoyListener)

	// Create Envoy Clusters per K8s Service
	for _, obj := range serviceK8sCacheStore.List() {
		service := obj.(*v1.Service)
		// create cluster only for node port type
		if service.Spec.Type == v1.ServiceTypeNodePort {
			for _, servicePort := range service.Spec.Ports {
				envoyCluster := v2.Cluster{
					Name:           service.Namespace + "--" + service.Name + "--" + fmt.Sprint(servicePort.Port),
					ConnectTimeout: time.Second * 1,
					LbPolicy:       v2.Cluster_ROUND_ROBIN,
					Type:           v2.Cluster_EDS,
					EdsClusterConfig: &v2.Cluster_EdsClusterConfig{
						EdsConfig: &core.ConfigSource{
							ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
								ApiConfigSource: &core.ApiConfigSource{
									ApiType:      core.ApiConfigSource_GRPC,
									GrpcServices: grpcServices,
								},
							},
						},
					},
				}

				envoyClusters = append(envoyClusters, &envoyCluster)

				//TODO fix the endpoints part

				lbEndpoints := []endpoint.LbEndpoint{}
				for _, obj := range nodeK8sCacheStore.List() {
					node := obj.(*v1.Node)
					lbEndpoint := endpoint.LbEndpoint{
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

					lbEndpoints = append(lbEndpoints, lbEndpoint)
				}

				envoyEndpoint := v2.ClusterLoadAssignment{
					ClusterName: service.Namespace + "--" + service.Name + "--" + fmt.Sprint(servicePort.Port),
					Endpoints: []endpoint.LocalityLbEndpoints{{
						LbEndpoints: lbEndpoints,
					}},
				}

				envoyEndpoints = append(envoyEndpoints, &envoyEndpoint)
			}
		}
	}

	log.Infof(">>>>>>>>>>>>>>>>>>> creating snapshot Version " + fmt.Sprint(version))

	snap := envoycache.NewSnapshot(fmt.Sprint(version), envoyEndpoints, envoyClusters, nil, envoyListeners)

	envoySnapshotCache.SetSnapshot("test-id", snap)
}

func makeConnectionManager(virtualHosts []route.VirtualHost) *hcm.HttpConnectionManager {
	//accessLogConfig, err := util.MessageToStruct(&fal.FileAccessLog{
	//	Path:   "/var/log/envoy/access.log",
	//	Format: jsonFormat,
	//})
	//if err != nil {
	//	log.Fatalf("failed to convert: %s", err)
	//}
	return &hcm.HttpConnectionManager{
		CodecType:  hcm.AUTO,
		StatPrefix: "ingress_http",
		HttpFilters: []*hcm.HttpFilter{&hcm.HttpFilter{
			Name: "envoy.router",
		}},
		UpgradeConfigs: []*hcm.HttpConnectionManager_UpgradeConfig{
			{
				UpgradeType: "websocket",
			},
		},
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: &v2.RouteConfiguration{
				Name:         "local_route",
				VirtualHosts: virtualHosts,
			},
		},
		Tracing: &hcm.HttpConnectionManager_Tracing{
			OperationName: hcm.EGRESS,
		},
		//AccessLog: []*al.AccessLog{
		//	{
		//		Name:   "envoy.file_access_log",
		//		Config: accessLogConfig,
		//	},
		//},
	}
}

func makeVirtualHost(namespace string, ingressRule extbeta1.IngressRule) route.VirtualHost {

	routes := []route.Route{}

	for _, httpPath := range ingressRule.HTTP.Paths {
		service, exists, _ := serviceK8sCacheStore.GetByKey(namespace + "/" + httpPath.Backend.ServiceName)
		if exists {
			k8sService := service.(*v1.Service)
			if k8sService.Spec.Type == v1.ServiceTypeNodePort {
				route := route.Route{
					Match: route.RouteMatch{
						PathSpecifier: &route.RouteMatch_Prefix{
							Prefix: httpPath.Path,
						},
					},
					Action: &route.Route_Route{
						Route: &route.RouteAction{
							//Timeout: &vhost.Timeout,
							ClusterSpecifier: &route.RouteAction_Cluster{
								Cluster: namespace + "--" + httpPath.Backend.ServiceName + "--" + fmt.Sprint(httpPath.Backend.ServicePort.IntVal),
							},
							//RetryPolicy: &route.RetryPolicy {
							//	RetryOn:       "5xx",
							//	PerTryTimeout: time.Second * 20,
							//},
						},
					},
				}

				routes = append(routes, route)
			}
		}
	}

	virtualHost := route.VirtualHost{
		Name:    "local_service",
		Domains: []string{ingressRule.Host},
		Routes:  routes,
	}
	return virtualHost
}