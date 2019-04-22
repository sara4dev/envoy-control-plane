package envoy

import (
	"crypto/tls"
	"git.target.com/Kubernetes/envoy-control-plane/pkg/data"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	al "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	fal "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	buf "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/buffer/v2"
	gzip "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/gzip/v2"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	"github.com/gogo/protobuf/types"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"strings"
	"time"
)

func (e *EnvoyCluster) makeEnvoyListeners(envoyListenersChan chan []cache.Resource) {
	start := time.Now()
	envoyListeners := []cache.Resource{}

	envoyHttpsListenersChan := make(chan []cache.Resource)
	envoyHttpListenersChan := make(chan []cache.Resource)

	go e.makeEnvoyHttpsListerners(envoyHttpsListenersChan)
	go e.makeEnvoyHttpListerners(envoyHttpListenersChan)

	envoyHttpsListeners := <-envoyHttpsListenersChan
	envoyListeners = append(envoyListeners, envoyHttpsListeners...)

	envoyHttpListeners := <-envoyHttpListenersChan
	envoyListeners = append(envoyListeners, envoyHttpListeners...)

	envoyListenersChan <- envoyListeners

	elapsed := time.Since(start)
	log.Debugf("makeEnvoyListeners took %v", elapsed)
}

func (e *EnvoyCluster) makeEnvoyHttpListerners(envoyHttpListenersChan chan []cache.Resource) {
	envoyListeners := []cache.Resource{}
	virtualHosts := []route.VirtualHost{}
	virtualHostsMap := make(map[string]route.VirtualHost)
	for _, k8sCluster := range e.K8sCacheStoreMap {
		for _, ingressObj := range k8sCluster.IngressCacheStore.List() {
			ingress := ingressObj.(*v1beta1.Ingress)
			// add it to HTTP listener only if ingress has no TLS
			if len(ingress.Spec.TLS) == 0 {
				for _, ingressRule := range ingress.Spec.Rules {
					virtualHost := makeVirtualHost(k8sCluster, ingress.Namespace, ingressRule, "80")
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

	httpConnectionManager := makeConnectionManager(virtualHosts, "ingress_http")
	httpConfig, err := types.MarshalAny(httpConnectionManager)
	if err != nil {
		log.Fatal("Error in converting connection manager")
	}

	filterChain := e.makeEnvoyListenerFilterChain(httpConfig, nil, nil)
	envoyListener := e.makeEnvoyListener("http", 80, []listener.FilterChain{filterChain})
	envoyListeners = append(envoyListeners, envoyListener)
	envoyHttpListenersChan <- envoyListeners
}

func (e *EnvoyCluster) makeEnvoyListener(name string, port uint32, filterChains []listener.FilterChain) *v2.Listener {
	return &v2.Listener{
		Name: name,
		Address: core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Address: "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		},
		FilterChains: filterChains,
	}
}

func (e *EnvoyCluster) makeEnvoyListenerFilterChain(httpConfig *types.Any, tlsContext *auth.DownstreamTlsContext, filterChainMatch *listener.FilterChainMatch) listener.FilterChain {
	return listener.FilterChain{
		TlsContext:       tlsContext,
		FilterChainMatch: filterChainMatch,
		Filters: []listener.Filter{
			{
				Name: util.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: httpConfig,
				},
			},
		},
	}
}

func (e *EnvoyCluster) makeEnvoyHttpsListerners(envoyHttpsListenersChan chan []cache.Resource) {
	envoyListeners := []cache.Resource{}
	listenerFilerChains := []listener.FilterChain{}
	listenerFilerChainsMap := make(map[string]listener.FilterChain)
	for _, k8sCluster := range e.K8sCacheStoreMap {
		for _, ingressObj := range k8sCluster.IngressCacheStore.List() {
			ingress := ingressObj.(*v1beta1.Ingress)
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
						makeVirtualHost(k8sCluster, ingress.Namespace, ingressRule, "443"),
					}
					httpConnectionManager := makeConnectionManager(virtualHosts, "ingress_https")
					httpConfig, err := types.MarshalAny(httpConnectionManager)
					if err != nil {
						log.Fatal("Error in converting connection manager")
					}

					tlsContext := e.getTLS(k8sCluster, ingress.Namespace, tlsSecretsMap[ingressRule.Host])
					filterChainMatch := &listener.FilterChainMatch{
						ServerNames: []string{ingressRule.Host},
					}
					filterChain := e.makeEnvoyListenerFilterChain(httpConfig, tlsContext, filterChainMatch)

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
	envoyListener := e.makeEnvoyListener("https", 443, listenerFilerChains)
	envoyListeners = append(envoyListeners, envoyListener)
	envoyHttpsListenersChan <- envoyListeners
}

func makeConnectionManager(virtualHosts []route.VirtualHost, statPrefix string) *hcm.HttpConnectionManager {
	//jsonFormat := make(map[string]*types.Value)
	//jsonFormat["protocol"] = &types.Value{ Kind:&types.Value_StringValue{StringValue: "%PROTOCOL%"}}
	accessLogConfig, err := types.MarshalAny(&al.FileAccessLog{
		Path: "/var/log/access.log",
		//AccessLogFormat: &al.FileAccessLog_JsonFormat{
		//	JsonFormat: &types.Struct{
		//		Fields: jsonFormat,
		//	},
		//},
	})

	if err != nil {
		log.Fatalf("failed to convert: %s", err)
	}

	httpBuffer, err := types.MarshalAny(&buf.Buffer{
		MaxRequestBytes: &types.UInt32Value{Value: 1024 * 1024 * 1024},
	})

	if err != nil {
		log.Fatalf("failed to convert: %s", err)
	}

	gzip, err := types.MarshalAny(&gzip.Gzip{
		MemoryLevel: &types.UInt32Value{Value: 9},
	})

	if err != nil {
		log.Fatalf("failed to convert: %s", err)
	}

	requestTimeout := 5 * time.Minute
	return &hcm.HttpConnectionManager{
		CodecType:      hcm.AUTO,
		StatPrefix:     statPrefix,
		RequestTimeout: &requestTimeout,
		HttpFilters: []*hcm.HttpFilter{
			{
				Name: util.Router,
			},
			{
				Name: util.Buffer,
				ConfigType: &hcm.HttpFilter_TypedConfig{
					TypedConfig: httpBuffer,
				},
			},
			{
				Name: util.Gzip,
				ConfigType: &hcm.HttpFilter_TypedConfig{
					TypedConfig: gzip,
				},
			},
		},
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
		AccessLog: []*fal.AccessLog{
			{
				Name: "envoy.file_access_log",
				ConfigType: &fal.AccessLog_TypedConfig{
					TypedConfig: accessLogConfig,
				},
			},
		},
	}
}

func findService(k8sCluster *data.K8sCacheStore, namespace string, serviceName string) *v1.Service {
	for _, serviceObj := range k8sCluster.ServiceCacheStore.List() {
		service := serviceObj.(*v1.Service)
		if service.Namespace == namespace && service.Name == serviceName {
			return service
		}
	}
	return nil
}

func makeVirtualHost(k8sCluster *data.K8sCacheStore, namespace string, ingressRule v1beta1.IngressRule, portNumber string) route.VirtualHost {

	routes := []route.Route{}

	for _, httpPath := range ingressRule.HTTP.Paths {
		service := findService(k8sCluster, namespace, httpPath.Backend.ServiceName)
		if service != nil {
			if service.Spec.Type == v1.ServiceTypeNodePort {
				route := makeRoute(httpPath, namespace, ingressRule)
				routes = append(routes, route)
			}
		}
	}

	virtualHost := route.VirtualHost{
		Name:    "local_service",
		Domains: []string{ingressRule.Host, ingressRule.Host + ":" + portNumber},
		Routes:  routes,
	}
	return virtualHost
}

func makeRoute(httpPath v1beta1.HTTPIngressPath, namespace string, ingressRule v1beta1.IngressRule) route.Route {
	perTryTimeout := time.Second * 20
	numRetries := types.UInt32Value{Value: 1}

	return route.Route{
		Match: route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: httpPath.Path,
			},
		},
		Action: &route.Route_Route{
			Route: &route.RouteAction{
				ClusterSpecifier: &route.RouteAction_Cluster{
					Cluster: getClusterName(namespace, ingressRule.Host, httpPath.Backend.ServiceName, httpPath.Backend.ServicePort.IntVal),
				},
				RetryPolicy: &route.RetryPolicy{
					RetryOn:       "5xx",
					NumRetries:    &numRetries,
					PerTryTimeout: &perTryTimeout,
				},
			},
		},
	}
}

func (e *EnvoyCluster) getTLS(k8sCluster *data.K8sCacheStore, namespace string, tlsSecretName string) *auth.DownstreamTlsContext {
	defaultTLSSecretNamespace, defaultTLSSecretName := e.getDefaultTLSSecret()
	tls := &auth.DownstreamTlsContext{}
	tls.CommonTlsContext = &auth.CommonTlsContext{
		TlsCertificates: []*auth.TlsCertificate{},
	}

	if tlsSecretName != "" {
		tls.CommonTlsContext.TlsCertificates = []*auth.TlsCertificate{e.getTLSData(k8sCluster, namespace, tlsSecretName)}
	} else {
		tls.CommonTlsContext.TlsCertificates = []*auth.TlsCertificate{e.getTLSData(k8sCluster, defaultTLSSecretNamespace, defaultTLSSecretName)}
	}

	return tls
}

func (e *EnvoyCluster) getTLSData(k8sCluster *data.K8sCacheStore, namespace string, tlsSecretName string) *auth.TlsCertificate {
	defaultTLSSecretNamespace, defaultTLSSecretName := e.getDefaultTLSSecret()
	key := k8sCluster.Name + "--" + namespace + "--" + tlsSecretName
	tlsCertificate := auth.TlsCertificate{}
	value, ok := e.tlsDataCache.Load(key)
	if ok {
		tlsCertificate = value.(auth.TlsCertificate)
	} else {
		defaultTLS, exists, err := k8sCluster.SecretCacheStore.GetByKey(namespace + "/" + tlsSecretName)
		if err != nil {
			log.Warn("Error in finding TLS secrets:" + namespace + "-" + tlsSecretName + ", using the default certs")
			return e.getTLSData(k8sCluster, defaultTLSSecretNamespace, defaultTLSSecretName)
		}
		if exists {
			defaultTLSSecret := defaultTLS.(*v1.Secret)
			certPem := []byte(defaultTLSSecret.Data["tls.crt"])
			keyPem := []byte(defaultTLSSecret.Data["tls.key"])

			_, err = tls.X509KeyPair(certPem, keyPem)
			if err != nil {
				log.Warn("Bad certificate in " + namespace + "-" + tlsSecretName + ", using the default certs")
				return e.getTLSData(k8sCluster, defaultTLSSecretNamespace, defaultTLSSecretName)
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
			e.tlsDataCache.Store(key, tlsCertificate)
		}
	}
	return &tlsCertificate
}

func (e *EnvoyCluster) getDefaultTLSSecret() (namespace string, name string) {
	namespacedName := strings.Split(e.DefaultTlsSecret, "/")
	return namespacedName[0], namespacedName[1]
}
