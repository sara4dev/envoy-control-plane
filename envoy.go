package main

import (
	"crypto/tls"
	"fmt"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const grpcMaxConcurrentStreams = 2147483647

type EnvoyCluster struct {
	envoySnapshotCache envoycache.SnapshotCache
	version            int32
	tlsDataCache       sync.Map
}

type k8sService struct {
	name        string
	ingressName string
	namespace   string
	port        int32
}

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

func (e *EnvoyCluster) RunManagementServer(ctx context.Context, port uint) {
	// gRPC golang library sets a very small upper bound for the number gRPC/h2
	// streams over a single TCP connection. If a proxy multiplexes requests over
	// a single connection to the management server, then it might lead to
	// availability problems.
	var grpcOptions []grpc.ServerOption
	signal := make(chan struct{})
	cb := &callbacks{signal: signal}
	server := server.NewServer(e.envoySnapshotCache, cb)
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

func (envoyCluster *EnvoyCluster) createEnvoySnapshot() {
	atomic.AddInt32(&envoyCluster.version, 1)
	//nodeId := envoySnapshotCache.GetStatusKeys()[0]
	log.Infof(">>>>>>>>>>>>>>>>>>> creating snapshot Version " + fmt.Sprint(envoyCluster.version))

	envoyListenersChan := make(chan []envoycache.Resource)
	envoyClustersChan := make(chan []envoycache.Resource)
	envoyEndpointsChan := make(chan []envoycache.Resource)

	go makeEnvoyListeners(envoyListenersChan)
	go makeEnvoyClusters(envoyClustersChan)
	go makeEnvoyEndpoints(envoyEndpointsChan)

	envoyListeners := <-envoyListenersChan
	envoyEndpoints := <-envoyEndpointsChan
	envoyClusters := <-envoyClustersChan

	snap := envoycache.NewSnapshot(fmt.Sprint(envoyCluster.version), envoyEndpoints, envoyClusters, nil, envoyListeners)

	envoyCluster.envoySnapshotCache.SetSnapshot("k8s_ingress", snap)
}

func makeEnvoyClusters(envoyClustersChan chan []envoycache.Resource) {
	envoyClusters := []envoycache.Resource{}

	grpcServices := []*core.GrpcService{}
	grpcService := &core.GrpcService{
		TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
				ClusterName: "xds_cluster",
			},
		},
	}
	grpcServices = append(grpcServices, grpcService)

	clusterMap := make(map[string]string)
	// Create Envoy Clusters per K8s Service referenced in ingress

	for _, k8sCluster := range k8sClusters {
		for _, obj := range k8sCluster.ingressCacheStore.List() {
			ingress := obj.(*extbeta1.Ingress)
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
		envoyCluster := v2.Cluster{
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
		envoyClusters = append(envoyClusters, &envoyCluster)
	}

	envoyClustersChan <- envoyClusters
}

func makeEnvoyEndpoints(envoyEndpointsChan chan []envoycache.Resource) {
	envoyEndpoints := []envoycache.Resource{}
	localityLbEndpointsMap := make(map[string][]endpoint.LocalityLbEndpoints)

	clusterMap := make(map[string]k8sService)
	// Create Envoy Clusters per K8s Service referenced in ingress

	for _, k8sCluster := range k8sClusters {
		for _, obj := range k8sCluster.ingressCacheStore.List() {
			ingress := obj.(*extbeta1.Ingress)
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

	for _, k8sCluster := range k8sClusters {
		for clusterName, k8sService := range clusterMap {
			lbEndpoints := []endpoint.LbEndpoint{}
			localityLbEndpoint := endpoint.LocalityLbEndpoints{}
			serviceObj, serviceExists, err := k8sCluster.serviceCacheStore.GetByKey(k8sService.namespace + "/" + k8sService.name)
			if err != nil {
				log.Fatal("Error in getting service by name")
			}
			_, ingressExists, err := k8sCluster.ingressCacheStore.GetByKey(k8sService.namespace + "/" + k8sService.ingressName)
			if err != nil {
				log.Fatal("Error in getting ingress by name")
			}
			if serviceExists && ingressExists {
				service := serviceObj.(*v1.Service)
				if service.Spec.Type == v1.ServiceTypeNodePort {
					for _, servicePort := range service.Spec.Ports {
						for _, nodeObj := range k8sCluster.nodeCacheStore.List() {
							node := nodeObj.(*v1.Node)
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
					}
				}

				localityLbEndpoint = endpoint.LocalityLbEndpoints{
					Locality: &core.Locality{
						Zone: strconv.Itoa(int(k8sCluster.zone)),
					},
					Priority:    k8sCluster.priority,
					LbEndpoints: lbEndpoints,
				}
				localityLbEndpointsMap[clusterName] = append(localityLbEndpointsMap[clusterName], localityLbEndpoint)
			}
		}
	}

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

	envoyEndpointsChan <- envoyEndpoints
}

func getClusterName(k8sNamespace string, k8singressHost string, k8sServiceName string, k8sServicePort int32) string {
	return k8sNamespace + ":" + k8singressHost + ":" + k8sServiceName + ":" + fmt.Sprint(k8sServicePort)
	//return k8sNamespace + ":" + k8sServiceName + ":" + fmt.Sprint(k8sServicePort)
}

func getTLS(k8sCluster *k8sCluster, namespace string, tlsSecretName string) *auth.DownstreamTlsContext {
	tls := &auth.DownstreamTlsContext{}
	tls.CommonTlsContext = &auth.CommonTlsContext{
		TlsCertificates: []*auth.TlsCertificate{},
	}

	if tlsSecretName != "" {
		tls.CommonTlsContext.TlsCertificates = []*auth.TlsCertificate{getTLSData(k8sCluster, namespace, tlsSecretName)}
	} else {
		//TODO remove hard coding of default TLS
		tls.CommonTlsContext.TlsCertificates = []*auth.TlsCertificate{getTLSData(k8sCluster, "kube-system", "haproxy-ingress-np-tls-secret")}
	}

	return tls
}

func getTLSData(k8sCluster *k8sCluster, namespace string, tlsSecretName string) *auth.TlsCertificate {
	key := k8sCluster.name + "--" + namespace + "--" + tlsSecretName
	tlsCertificate := auth.TlsCertificate{}
	value, ok := envoyCluster.tlsDataCache.Load(key)
	if ok {
		tlsCertificate = value.(auth.TlsCertificate)
	} else {
		defaultTLS, exists, err := k8sCluster.secretCacheStore.GetByKey(namespace + "/" + tlsSecretName)
		if err != nil {
			log.Warn("Error in finding TLS secrets:" + namespace + "-" + tlsSecretName + ", using the default certs")
			//TODO remove hard coding of default TLS
			return getTLSData(k8sCluster, "kube-system", "haproxy-ingress-np-tls-secret")
		}
		if exists {
			defaultTLSSecret := defaultTLS.(*v1.Secret)
			certPem := []byte(defaultTLSSecret.Data["tls.crt"])
			keyPem := []byte(defaultTLSSecret.Data["tls.key"])

			_, err = tls.X509KeyPair(certPem, keyPem)
			if err != nil {
				log.Warn("Bad certificate in " + namespace + "-" + tlsSecretName + ", using the default certs")
				//TODO remove hard coding of default TLS
				return getTLSData(k8sCluster, "kube-system", "haproxy-ingress-np-tls-secret")
			}

			tlsCertificate = auth.TlsCertificate{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(defaultTLSSecret.Data["tls.crt"]),
					},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_InlineBytes{
						InlineBytes: []byte(defaultTLSSecret.Data["tls.key"]),
					},
				},
			}

			//TODO how to update the cache if the TLS changes?
			envoyCluster.tlsDataCache.Store(key, tlsCertificate)
		}
	}
	return &tlsCertificate
}

func makeEnvoyListeners(envoyListenersChan chan []envoycache.Resource) {
	start := time.Now()
	envoyListeners := []envoycache.Resource{}

	envoyHttpsListenersChan := make(chan []envoycache.Resource)
	envoyHttpListenersChan := make(chan []envoycache.Resource)

	go makeEnvoyHttpsListerners(envoyHttpsListenersChan)
	go makeEnvoyHttpListerners(envoyHttpListenersChan)

	envoyHttpsListeners := <-envoyHttpsListenersChan
	envoyListeners = append(envoyListeners, envoyHttpsListeners...)

	envoyHttpListeners := <-envoyHttpListenersChan
	envoyListeners = append(envoyListeners, envoyHttpListeners...)

	envoyListenersChan <- envoyListeners

	elapsed := time.Since(start)
	log.Printf("makeEnvoyListeners took %s", elapsed)
}

func makeEnvoyHttpListerners(envoyHttpListenersChan chan []envoycache.Resource) {
	envoyListeners := []envoycache.Resource{}
	virtualHosts := []route.VirtualHost{}
	virtualHostsMap := make(map[string]route.VirtualHost)
	for _, k8sCluster := range k8sClusters {
		for _, ingressObj := range k8sCluster.ingressCacheStore.List() {
			ingress := ingressObj.(*extbeta1.Ingress)
			// add it to HTTP listener only if ingress has no TLS
			if len(ingress.Spec.TLS) == 0 {
				for _, ingressRule := range ingress.Spec.Rules {
					virtualHost := makeVirtualHost(k8sCluster, ingress.Namespace, ingressRule)
					existingVirtualHost := virtualHostsMap[ingressRule.Host]
					if existingVirtualHost.Name != "" {
						existingVirtualHost.Routes = append(existingVirtualHost.Routes, virtualHost.Routes...)
					} else {
						virtualHostsMap[ingressRule.Host] = virtualHost
					}
				}
			}
		}
	}
	for _, value := range virtualHostsMap {
		virtualHosts = append(virtualHosts, value)
	}

	httpConnectionManager := makeConnectionManager(virtualHosts)
	httpConfig, err := types.MarshalAny(httpConnectionManager)
	if err != nil {
		log.Fatal("Error in converting connection manager")
	}

	filterChain := listener.FilterChain{
		Filters: []listener.Filter{
			{
				Name: util.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: httpConfig,
				},
			},
		},
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
		FilterChains: []listener.FilterChain{filterChain},
	}
	envoyListeners = append(envoyListeners, envoyListener)
	envoyHttpListenersChan <- envoyListeners
}

func makeEnvoyHttpsListerners(envoyHttpsListenersChan chan []envoycache.Resource) {
	envoyListeners := []envoycache.Resource{}
	listenerFilerChains := []listener.FilterChain{}
	listenerFilerChainsMap := make(map[string]listener.FilterChain)
	for _, k8sCluster := range k8sClusters {
		for _, ingressObj := range k8sCluster.ingressCacheStore.List() {
			ingress := ingressObj.(*extbeta1.Ingress)
			// add it to HTTPS listener only if ingress has TLS
			if len(ingress.Spec.TLS) > 0 {
				tlsSecretsMap := make(map[string]string)
				for _, tlsCerts := range ingress.Spec.TLS {
					for _, host := range tlsCerts.Hosts {
						tlsSecretsMap[host] = tlsCerts.SecretName
					}
				}

				for _, ingressRule := range ingress.Spec.Rules {

					virtualHosts := []route.VirtualHost{
						makeVirtualHost(k8sCluster, ingress.Namespace, ingressRule),
					}
					httpConnectionManager := makeConnectionManager(virtualHosts)
					httpConfig, err := types.MarshalAny(httpConnectionManager)
					if err != nil {
						log.Fatal("Error in converting connection manager")
					}

					filterChain := listener.FilterChain{
						TlsContext: getTLS(k8sCluster, ingress.Namespace, tlsSecretsMap[ingressRule.Host]),
						FilterChainMatch: &listener.FilterChainMatch{
							ServerNames: []string{ingressRule.Host},
						},
						Filters: []listener.Filter{
							{
								Name: util.HTTPConnectionManager,
								ConfigType: &listener.Filter_TypedConfig{
									TypedConfig: httpConfig,
								},
							},
						},
					}

					existingFilterChain := listenerFilerChainsMap[ingressRule.Host]
					if existingFilterChain.FilterChainMatch != nil {
						// if the domain already exists, combine the routes
						existingHttpConnectionManager := &hcm.HttpConnectionManager{}
						err = types.UnmarshalAny(existingFilterChain.Filters[0].ConfigType.(*listener.Filter_TypedConfig).TypedConfig, existingHttpConnectionManager)
						if err != nil {
							log.Warn("Error in converting filter chain")
						}
						existingRoutes := existingHttpConnectionManager.RouteSpecifier.(*hcm.HttpConnectionManager_RouteConfig).RouteConfig.VirtualHosts[0].Routes
						existingRoutes = append(existingRoutes, virtualHosts[0].Routes...)
						existingHttpConnectionManager.RouteSpecifier.(*hcm.HttpConnectionManager_RouteConfig).RouteConfig.VirtualHosts[0].Routes = existingRoutes
					} else {
						listenerFilerChainsMap[ingressRule.Host] = filterChain
					}
				}
			}
		}
	}
	for _, value := range listenerFilerChainsMap {
		if len(value.TlsContext.CommonTlsContext.TlsCertificates) == 0 && value.TlsContext.CommonTlsContext.TlsCertificates[0].CertificateChain == nil {
			log.Fatal("No certificate found for " + value.FilterChainMatch.ServerNames[0])
		}
		listenerFilerChains = append(listenerFilerChains, value)
	}
	envoyListener := &v2.Listener{
		Name: "https",
		Address: core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Address: "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: 443,
					},
				},
			},
		},
		//TODO fix the route part
		FilterChains: listenerFilerChains,
	}
	envoyListeners = append(envoyListeners, envoyListener)
	envoyHttpsListenersChan <- envoyListeners
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

func findService(k8sCluster *k8sCluster, namespace string, serviceName string) *v1.Service {
	for _, serviceObj := range k8sCluster.serviceCacheStore.List() {
		service := serviceObj.(*v1.Service)
		if service.Namespace == namespace && service.Name == serviceName {
			return service
		}
	}
	return nil
}

func makeVirtualHost(k8sCluster *k8sCluster, namespace string, ingressRule extbeta1.IngressRule) route.VirtualHost {

	routes := []route.Route{}

	for _, httpPath := range ingressRule.HTTP.Paths {
		//log.Info("k8sCluster: " + strconv.Itoa(k8sCluster))
		service := findService(k8sCluster, namespace, httpPath.Backend.ServiceName)
		if service != nil {
			if service.Spec.Type == v1.ServiceTypeNodePort {
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
								Cluster: getClusterName(namespace, ingressRule.Host, httpPath.Backend.ServiceName, httpPath.Backend.ServicePort.IntVal),
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
