package main

//
//import (
//	"bytes"
//	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
//	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
//	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
//	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
//	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
//	"github.com/gogo/protobuf/types"
//	"k8s.io/api/core/v1"
//	"k8s.io/api/extensions/v1beta1"
//	k8scache "k8s.io/client-go/tools/cache"
//	"log"
//	"testing"
//)
//
//type k8sTestData struct {
//	clusterName string
//	ingressList v1beta1.IngressList
//	serviceList v1.ServiceList
//	secretList  v1.SecretList
//	nodeList    v1.NodeList
//}
//
//var k8sTestDataMap map[string]*k8sTestData
//var testEnvoyCluster EnvoyCluster
//
//// Test Data Setup
//
//func setupEnvoyTest() {
//
//	testEnvoyCluster = EnvoyCluster{}
//	testEnvoyCluster.k8sCacheStoreMap = make(map[string]*K8sCacheStore)
//	testEnvoyCluster.k8sCacheStoreMap["cluster1"] = &K8sCacheStore{
//		Name:     "cluster1",
//		Zone:     TTC,
//		Priority: 0,
//	}
//
//	testEnvoyCluster.k8sCacheStoreMap["cluster2"] = &K8sCacheStore{
//		Name:     "cluster2",
//		Zone:     TTE,
//		Priority: 1,
//	}
//
//	k8sTestDataMap = make(map[string]*k8sTestData)
//	for _, k8sCacheStore := range testEnvoyCluster.k8sCacheStoreMap {
//		k8sTestDataMap[k8sCacheStore.Name] = &k8sTestData{
//			clusterName: k8sCacheStore.Name,
//		}
//	}
//
//	for _, k8sTestData := range k8sTestDataMap {
//		k8sCacheStore := testEnvoyCluster.k8sCacheStoreMap[k8sTestData.clusterName]
//		loadTestData(k8sTestData)
//		k8sCacheStore.IngressCacheStore = &k8scache.FakeCustomStore{
//			ListKeysFunc: k8sTestData.fakeIngressKeys,
//			ListFunc:     k8sTestData.fakeIngresses,
//			GetByKeyFunc: k8sTestData.fakeIngressByKey,
//		}
//		//k8sCluster.initialIngresses = testEnvoyCluster.k8sCacheStoreMap[k8sCluster.name].IngressCacheStore.ListKeys()
//		k8sCacheStore.ServiceCacheStore = &k8scache.FakeCustomStore{
//			ListKeysFunc: k8sTestData.fakeServiceKeys,
//			ListFunc:     k8sTestData.fakeServices,
//			GetByKeyFunc: k8sTestData.fakeServiceByKey,
//		}
//		//k8sCluster.initialServices = testEnvoyCluster.k8sCacheStoreMap[k8sCluster.name].ServiceCacheStore.ListKeys()
//		k8sCacheStore.SecretCacheStore = &k8scache.FakeCustomStore{
//			ListKeysFunc: k8sTestData.fakeSecretKeys,
//			ListFunc:     k8sTestData.fakeSecrets,
//			GetByKeyFunc: k8sTestData.fakeSecretByKey,
//		}
//		//k8sCluster.initialSecrets = testEnvoyCluster.k8sCacheStoreMap[k8sCluster.name].SecretCacheStore.ListKeys()
//		k8sCacheStore.NodeCacheStore = &k8scache.FakeCustomStore{
//			ListKeysFunc: k8sTestData.fakeNodeKeys,
//			ListFunc:     k8sTestData.fakeNodes,
//			GetByKeyFunc: k8sTestData.fakeNodeByKey,
//		}
//		//k8sCluster.initialNodes = testEnvoyCluster.k8sCacheStoreMap[k8sCluster.name].NodeCacheStore.ListKeys()
//	}
//}
//
//func loadTestData(k8sTestData *k8sTestData) {
//	//load fake ingresses
//	loadObjFromFile("test-data/"+k8sTestData.clusterName+"-ingressList.yml", &k8sTestData.ingressList)
//
//	//load fake services
//	loadObjFromFile("test-data/"+k8sTestData.clusterName+"-serviceList.yml", &k8sTestData.serviceList)
//
//	//load fake secrets
//	loadObjFromFile("test-data/"+k8sTestData.clusterName+"-secretList.yml", &k8sTestData.secretList)
//
//	//load fake nodes
//	loadObjFromFile("test-data/"+k8sTestData.clusterName+"-nodeList.yml", &k8sTestData.nodeList)
//}
//
//func (c *k8sTestData) fakeIngresses() []interface{} {
//	fakeIngresses := c.ingressList.Items
//	ingressObjects := make([]interface{}, 0, len(fakeIngresses))
//	for i := 0; i < len(fakeIngresses); i++ {
//		ingressObjects = append(ingressObjects, &fakeIngresses[i])
//	}
//	return ingressObjects
//}
//
//func (c *k8sTestData) fakeServices() []interface{} {
//	fakeServices := c.serviceList.Items
//	serviceObjects := make([]interface{}, 0, len(fakeServices))
//	for i := 0; i < len(fakeServices); i++ {
//		serviceObjects = append(serviceObjects, &fakeServices[i])
//	}
//	return serviceObjects
//}
//
//func (c *k8sTestData) fakeSecrets() []interface{} {
//	fakeSecrets := c.secretList.Items
//	secretObjects := make([]interface{}, 0, len(fakeSecrets))
//	for i := 0; i < len(fakeSecrets); i++ {
//		secretObjects = append(secretObjects, &fakeSecrets[i])
//	}
//	return secretObjects
//}
//
//func (c *k8sTestData) fakeNodes() []interface{} {
//	fakeNodes := c.nodeList.Items
//	nodeObjects := make([]interface{}, 0, len(fakeNodes))
//	for i := 0; i < len(fakeNodes); i++ {
//		nodeObjects = append(nodeObjects, &fakeNodes[i])
//	}
//	return nodeObjects
//}
//
//func (c *k8sTestData) fakeIngressByKey(key string) (interface{}, bool, error) {
//	for _, ingress := range c.ingressList.Items {
//		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
//		if err != nil {
//			log.Fatal("Error while splittig the metanamespace key")
//		}
//		if ingress.Namespace == namespace && ingress.Name == name {
//			return &ingress, true, nil
//		}
//	}
//	return nil, false, nil
//}
//
//func (c *k8sTestData) fakeSecretByKey(key string) (interface{}, bool, error) {
//	for _, secret := range c.secretList.Items {
//		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
//		if err != nil {
//			log.Fatal("Error while splittig the metanamespace key")
//		}
//		if secret.Namespace == namespace && secret.Name == name {
//			return &secret, true, nil
//		}
//	}
//	return nil, false, nil
//}
//
//func (c *k8sTestData) fakeServiceByKey(key string) (interface{}, bool, error) {
//	for _, service := range c.serviceList.Items {
//		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
//		if err != nil {
//			log.Fatal("Error while splittig the metanamespace key")
//		}
//		if service.Namespace == namespace && service.Name == name {
//			return &service, true, nil
//		}
//	}
//	return nil, false, nil
//}
//
//func (c *k8sTestData) fakeNodeByKey(key string) (interface{}, bool, error) {
//	for _, node := range c.nodeList.Items {
//		if node.Name == key {
//			return &node, true, nil
//		}
//	}
//	return nil, false, nil
//}
//
//func (c *k8sTestData) fakeIngressKeys() []string {
//	keys := []string{}
//	for _, obj := range c.ingressList.Items {
//		keys = append(keys, obj.Namespace+"/"+obj.Name)
//	}
//	return keys
//}
//
//func (c *k8sTestData) fakeServiceKeys() []string {
//	keys := []string{}
//	for _, obj := range c.serviceList.Items {
//		keys = append(keys, obj.Namespace+"/"+obj.Name)
//	}
//	return keys
//}
//
//func (c *k8sTestData) fakeSecretKeys() []string {
//	keys := []string{}
//	for _, obj := range c.secretList.Items {
//		keys = append(keys, obj.Namespace+"/"+obj.Name)
//	}
//	return keys
//}
//
//func (c *k8sTestData) fakeNodeKeys() []string {
//	keys := []string{}
//	for _, obj := range c.nodeList.Items {
//		keys = append(keys, obj.Name)
//	}
//	return keys
//}
//
//// Tests
//
//func TestMakeEnvoyClusters(t *testing.T) {
//	setupEnvoyTest()
//	envoyClustersChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyClusters(envoyClustersChan)
//	envoyClusters := <-envoyClustersChan
//	if len(envoyClusters) != 15 {
//		t.Error("Unexpected number of Envoy Clusters")
//	}
//}
//
//func TestMakeEnvoyEndpoints_namespace1_http_cluster1_ingress1_service1_80(t *testing.T) {
//	//setupEnvoyTest()
//	envoyEndpointsChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyEndpoints(envoyEndpointsChan)
//	envoyEndpoints := <-envoyEndpointsChan
//
//	// endpoint object for every cluster, even for empty cluster
//	if len(envoyEndpoints) != 15 {
//		t.Error("Unexpected number of Envoy Endpoints")
//	}
//
//	matchedTestCluster := false
//	//endpoint for cluster cluster1: namespace1:http-ingress1.cluster1.k8s.io:service1:80
//	for _, envoyEndpointObj := range envoyEndpoints {
//		envoyEndpoint := envoyEndpointObj.(*v2.ClusterLoadAssignment)
//		//http-ingress1.cluster1.k8s.io
//		if envoyEndpoint.ClusterName == "namespace1:http-ingress1.cluster1.k8s.io:service1:80" {
//			matchedTestCluster = true
//			// only endpoint for cluster 1 should be created
//			if len(envoyEndpoint.Endpoints) > 1 {
//				t.Error("Unexpected number of Envoy Endpoints")
//			}
//			// only LBEndpoints for cluster 1 nodes should be created
//			if len(envoyEndpoint.Endpoints[0].LbEndpoints) != 5 {
//				t.Error("Unexpected number of Envoy LbEndpoints")
//			}
//		}
//	}
//
//	if !matchedTestCluster {
//		t.Error("No tests ran")
//	}
//
//}
//
//func TestMakeEnvoyEndpoints_namespace1_http_cluster2_ingress1_service1_80(t *testing.T) {
//	envoyEndpointsChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyEndpoints(envoyEndpointsChan)
//	envoyEndpoints := <-envoyEndpointsChan
//
//	matchedTestCluster := false
//	for _, envoyEndpointObj := range envoyEndpoints {
//		envoyEndpoint := envoyEndpointObj.(*v2.ClusterLoadAssignment)
//		if envoyEndpoint.ClusterName == "namespace1:http-ingress1.cluster2.k8s.io:service1:80" {
//			matchedTestCluster = true
//			// only endpoint for cluster 2 should be created
//			if len(envoyEndpoint.Endpoints) > 1 {
//				t.Error("Unexpected number of Envoy Endpoints")
//			}
//			// only LBEndpoints for cluster 2 nodes should be created
//			if len(envoyEndpoint.Endpoints[0].LbEndpoints) != 2 {
//				t.Error("Unexpected number of Envoy LbEndpoints")
//			}
//		}
//	}
//
//	if !matchedTestCluster {
//		t.Error("No tests ran")
//	}
//}
//
//func TestMakeEnvoyEndpoints_namespace2_http_cross_cluster_ingress_service2_80(t *testing.T) {
//	envoyEndpointsChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyEndpoints(envoyEndpointsChan)
//	envoyEndpoints := <-envoyEndpointsChan
//
//	matchedTestCluster := false
//	for _, envoyEndpointObj := range envoyEndpoints {
//		envoyEndpoint := envoyEndpointObj.(*v2.ClusterLoadAssignment)
//		//endpoint for cluster cluster1/2: namespace2:http-cross-cluster-ingress.k8s.io:service2:80 should be multi cluster ingress
//		if envoyEndpoint.ClusterName == "namespace2:http-cross-cluster-ingress.k8s.io:service2:80" {
//			matchedTestCluster = true
//			if len(envoyEndpoint.Endpoints) != 2 {
//				t.Error("Unexpected number of Envoy Endpoints")
//			}
//
//			// only LBEndpoints for cluster 1 nodes should be created
//			if len(envoyEndpoint.Endpoints[0].LbEndpoints) != 5 {
//				t.Error("Unexpected number of Envoy LbEndpoints")
//			}
//
//			// only LBEndpoints for cluster 2 nodes should be created
//			if len(envoyEndpoint.Endpoints[1].LbEndpoints) != 2 {
//				t.Error("Unexpected number of Envoy LbEndpoints")
//			}
//		}
//	}
//
//	if !matchedTestCluster {
//		t.Error("No tests ran")
//	}
//}
//
//func TestMakeEnvoyEndpoints_namespace6_http_clusterip_service_ingress_service6_80(t *testing.T) {
//	envoyEndpointsChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyEndpoints(envoyEndpointsChan)
//	envoyEndpoints := <-envoyEndpointsChan
//
//	matchedTestCluster := false
//	for _, envoyEndpointObj := range envoyEndpoints {
//		envoyEndpoint := envoyEndpointObj.(*v2.ClusterLoadAssignment)
//		//endpoint for cluster cluster1: namespace6:http-clusterip-service-ingress.cluster1.k8s.io:service6:80 should be 0 as it is not a nodeport service
//		if envoyEndpoint.ClusterName == "namespace6:http-clusterip-service-ingress.cluster1.k8s.io:service6:80" {
//			matchedTestCluster = true
//			for _, endPoint := range envoyEndpoint.Endpoints {
//				if len(endPoint.LbEndpoints) != 0 {
//					t.Error("Unexpected number of Envoy LbEndpoints")
//				}
//			}
//		}
//	}
//
//	if !matchedTestCluster {
//		t.Error("No tests ran")
//	}
//}
//
//func TestMakeEnvoyListeners(t *testing.T) {
//	//setupEnvoyTest()
//	envoyListenersChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyListeners(envoyListenersChan)
//	envoyListeners := <-envoyListenersChan
//	if len(envoyListeners) != 2 {
//		t.Error("Unexpected number of Envoy Listeners")
//	}
//}
//
//func TestMakeEnvoyListeners_http(t *testing.T) {
//	envoyListenersChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyListeners(envoyListenersChan)
//	envoyListeners := <-envoyListenersChan
//	matchedTestListener := false
//	for _, envoyListenerObj := range envoyListeners {
//		envoyListener := envoyListenerObj.(*v2.Listener)
//		if envoyListener.Name == "http" {
//			matchedTestListener = true
//			typedConfig := envoyListener.FilterChains[0].Filters[0].ConfigType.(*listener.Filter_TypedConfig)
//			httpConnectionManager := hcm.HttpConnectionManager{}
//			err = types.UnmarshalAny(typedConfig.TypedConfig, &httpConnectionManager)
//			if err != nil {
//				t.Error("Error in unmarshalling HttpConnectionManager")
//			}
//			httpConnectionManagerRouteConfig := httpConnectionManager.RouteSpecifier.(*hcm.HttpConnectionManager_RouteConfig)
//			if len(httpConnectionManagerRouteConfig.RouteConfig.VirtualHosts) != 8 {
//				t.Error("Unexpected number of Envoy HTTP Virtual Hosts")
//			}
//
//		}
//	}
//
//	if !matchedTestListener {
//		t.Error("No tests ran")
//	}
//
//}
//
//func TestMakeEnvoyListeners_https(t *testing.T) {
//	envoyListenersChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyListeners(envoyListenersChan)
//	envoyListeners := <-envoyListenersChan
//	matchedTestListener := false
//	for _, envoyListenerObj := range envoyListeners {
//		envoyListener := envoyListenerObj.(*v2.Listener)
//		if envoyListener.Name == "https" {
//			matchedTestListener = true
//			if len(envoyListener.FilterChains) != 3 {
//				t.Error("Unexpected number of Envoy HTTPS FilterChains")
//			}
//		}
//	}
//
//	if !matchedTestListener {
//		t.Error("No tests ran")
//	}
//}
//
//func TestMakeEnvoyListeners_https_invalid_tls_ingress(t *testing.T) {
//	envoyListenersChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyListeners(envoyListenersChan)
//	envoyListeners := <-envoyListenersChan
//	matchedTestListener := false
//	for _, envoyListenerObj := range envoyListeners {
//		envoyListener := envoyListenerObj.(*v2.Listener)
//		if envoyListener.Name == "https" {
//			for _, filterChain := range envoyListener.FilterChains {
//				if filterChain.FilterChainMatch.ServerNames[0] == "https-invalid-tls-ingress.cluster1.k8s.io" {
//					matchedTestListener = true
//					if len(filterChain.TlsContext.CommonTlsContext.TlsCertificates) != 1 {
//						t.Error("Unexpected number of Envoy HTTPS TlsCertificates")
//					}
//					//use default cert if found invalid cert
//					certificateChain := filterChain.TlsContext.CommonTlsContext.TlsCertificates[0].CertificateChain.Specifier.(*core.DataSource_InlineBytes)
//					if !bytes.Equal(certificateChain.InlineBytes, []byte(k8sTestDataMap["cluster1"].secretList.Items[2].Data["tls.crt"])) {
//						t.Error("Unexpected Envoy HTTPS TlsCertificate")
//					}
//					privateKey := filterChain.TlsContext.CommonTlsContext.TlsCertificates[0].PrivateKey.Specifier.(*core.DataSource_InlineBytes)
//					if !bytes.Equal(privateKey.InlineBytes, []byte(k8sTestDataMap["cluster1"].secretList.Items[2].Data["tls.key"])) {
//						t.Error("Unexpected Envoy HTTPS TlsCertificate key")
//					}
//				}
//			}
//		}
//	}
//
//	if !matchedTestListener {
//		t.Error("No tests ran")
//	}
//}
//
//func TestMakeEnvoyListeners_https_default_tls_ingress(t *testing.T) {
//	envoyListenersChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyListeners(envoyListenersChan)
//	envoyListeners := <-envoyListenersChan
//	matchedTestListener := false
//	for _, envoyListenerObj := range envoyListeners {
//		envoyListener := envoyListenerObj.(*v2.Listener)
//		if envoyListener.Name == "https" {
//			for _, filterChain := range envoyListener.FilterChains {
//				if filterChain.FilterChainMatch.ServerNames[0] == "https-default-tls-ingress.cluster1.k8s.io" {
//					matchedTestListener = true
//					if len(filterChain.TlsContext.CommonTlsContext.TlsCertificates) != 1 {
//						t.Error("Unexpected number of Envoy HTTPS TlsCertificates")
//					}
//					//use default cert as TLS secret is not mentioned in the ingress
//					certificateChain := filterChain.TlsContext.CommonTlsContext.TlsCertificates[0].CertificateChain.Specifier.(*core.DataSource_InlineBytes)
//					if !bytes.Equal(certificateChain.InlineBytes, []byte(k8sTestDataMap["cluster1"].secretList.Items[2].Data["tls.crt"])) {
//						t.Error("Unexpected Envoy HTTPS TlsCertificate")
//					}
//					privateKey := filterChain.TlsContext.CommonTlsContext.TlsCertificates[0].PrivateKey.Specifier.(*core.DataSource_InlineBytes)
//					if !bytes.Equal(privateKey.InlineBytes, []byte(k8sTestDataMap["cluster1"].secretList.Items[2].Data["tls.key"])) {
//						t.Error("Unexpected Envoy HTTPS TlsCertificate key")
//					}
//				}
//			}
//		}
//	}
//
//	if !matchedTestListener {
//		t.Error("No tests ran")
//	}
//}
//
//func TestMakeEnvoyListeners_https_valid_tls_ingress(t *testing.T) {
//	envoyListenersChan := make(chan []envoycache.Resource)
//	go testEnvoyCluster.makeEnvoyListeners(envoyListenersChan)
//	envoyListeners := <-envoyListenersChan
//	matchedTestListener := false
//	for _, envoyListenerObj := range envoyListeners {
//		envoyListener := envoyListenerObj.(*v2.Listener)
//		if envoyListener.Name == "https" {
//			for _, filterChain := range envoyListener.FilterChains {
//				if filterChain.FilterChainMatch.ServerNames[0] == "https-valid-tls-ingress.cluster1.k8s.io" {
//					matchedTestListener = true
//					if len(filterChain.TlsContext.CommonTlsContext.TlsCertificates) != 1 {
//						t.Error("Unexpected number of Envoy HTTPS TlsCertificates")
//					}
//					//use valid cert found in the ingress
//					certificateChain := filterChain.TlsContext.CommonTlsContext.TlsCertificates[0].CertificateChain.Specifier.(*core.DataSource_InlineBytes)
//					if !bytes.Equal(certificateChain.InlineBytes, []byte(k8sTestDataMap["cluster1"].secretList.Items[1].Data["tls.crt"])) {
//						t.Error("Unexpected Envoy HTTPS TlsCertificate")
//					}
//					privateKey := filterChain.TlsContext.CommonTlsContext.TlsCertificates[0].PrivateKey.Specifier.(*core.DataSource_InlineBytes)
//					if !bytes.Equal(privateKey.InlineBytes, []byte(k8sTestDataMap["cluster1"].secretList.Items[1].Data["tls.key"])) {
//						t.Error("Unexpected Envoy HTTPS TlsCertificate key")
//					}
//				}
//			}
//		}
//	}
//
//	if !matchedTestListener {
//		t.Error("No tests ran")
//	}
//}
