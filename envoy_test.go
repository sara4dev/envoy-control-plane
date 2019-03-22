package main

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/yaml"
	k8scache "k8s.io/client-go/tools/cache"
	"log"
	"os"
	"testing"
)

type k8sTestData struct {
	ingressList v1beta1.IngressList
	serviceList v1.ServiceList
	secretList  v1.SecretList
	nodeList    v1.NodeList
}

var k8sTestDataMap map[string]k8sTestData

func TestMakeEnvoyClusters(t *testing.T) {
	setupEnvoyTest()
	envoyClustersChan := make(chan []envoycache.Resource)
	go makeEnvoyClusters(envoyClustersChan)
	envoyClusters := <-envoyClustersChan
	if len(envoyClusters) != 13 {
		t.Error("Unexpected number of Envoy Clusters")
	}
}

func TestMakeEnvoyEndpoints(t *testing.T) {
	//setupEnvoyTest()
	envoyEndpointsChan := make(chan []envoycache.Resource)
	go makeEnvoyEndpoints(envoyEndpointsChan)
	envoyEndpoints := <-envoyEndpointsChan

	// endpoint object for every cluster, even for empty cluster
	if len(envoyEndpoints) != 13 {
		t.Error("Unexpected number of Envoy Endpoints")
	}

	// endpoint for cluster cluster1--namespace6--service6--80 should be 0 as it is not a nodeport service
	//for _, envoyEndpointObj := range envoyEndpoints {
	//	envoyEndpoint := envoyEndpointObj.(*v2.ClusterLoadAssignment)
	//	if envoyEndpoint.ClusterName == "cluster1--namespace6--service6--80" {
	//		if len(envoyEndpoint.Endpoints) != 0 {
	//			t.Error("Unexpected number of Envoy Endpoints")
	//		}
	//	}
	//}
}

//func TestMakeEnvoyListeners(t *testing.T) {
//	//setupEnvoyTest()
//	envoyListenersChan := make(chan []envoycache.Resource)
//	go makeEnvoyListeners(envoyListenersChan)
//	envoyListeners := <-envoyListenersChan
//	if len(envoyListeners) != 1 {
//		t.Error("No Envoy Listeners created")
//	}
//}

func setupEnvoyTest() {

	k8sClusters = []*k8sCluster{
		{
			name: "cluster1",
			zone: TTC,
		},
		{
			name: "cluster2",
			zone: TTE,
		},
	}

	k8sTestDataMap = make(map[string]k8sTestData)

	for _, k8sCluster := range k8sClusters {
		k8sCluster.loadTestData()
		k8sCluster.ingressCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: k8sCluster.fakeIngressKeys,
			ListFunc:     k8sCluster.fakeIngresses,
		}
		k8sCluster.serviceCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: k8sCluster.fakeServiceKeys,
			ListFunc:     k8sCluster.fakeServices,
			GetByKeyFunc: k8sCluster.fakeServiceByKey,
		}
		k8sCluster.secretCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: k8sCluster.fakeSecretKeys,
			ListFunc:     k8sCluster.fakeSecrets,
			GetByKeyFunc: k8sCluster.fakeSecretByKey,
		}
		k8sCluster.nodeCacheStore = &k8scache.FakeCustomStore{
			ListFunc: k8sCluster.fakeNodes,
		}
	}
}

func (c *k8sCluster) loadTestData() {
	k8sTestData := k8sTestData{}

	//load fake ingresses
	reader, err := os.Open("test-data/" + c.name + "-ingressList.yml")
	if err != nil {
		log.Fatal("Failed to setup fake Ingress")
	}
	err = yaml.NewYAMLOrJSONDecoder(reader, 2048).Decode(&k8sTestData.ingressList)
	if err != nil {
		log.Fatal("Failed to setup fake Ingress")
	}

	//load fake services
	reader, err = os.Open("test-data/" + c.name + "-serviceList.yml")
	if err != nil {
		log.Fatal("Failed to setup fake Service")
	}
	err = yaml.NewYAMLOrJSONDecoder(reader, 2048).Decode(&k8sTestData.serviceList)
	if err != nil {
		log.Fatal("Failed to setup fake Service")
	}

	//load fake secrets
	//reader, err = os.Open("test-data/" + c.name + "-secretList.yml")
	//if err != nil {
	//	log.Fatal("Failed to setup fake Secret")
	//}
	//err = yaml.NewYAMLOrJSONDecoder(reader, 2048).Decode(&k8sTestData.secretList)
	//if err != nil {
	//	log.Fatal("Failed to setup fake Secret")
	//}

	//load fake nodes
	reader, err = os.Open("test-data/" + c.name + "-nodeList.yml")
	if err != nil {
		log.Fatal("Failed to setup fake nodes")
	}
	err = yaml.NewYAMLOrJSONDecoder(reader, 2048).Decode(&k8sTestData.nodeList)
	if err != nil {
		log.Fatal("Failed to setup fake nodes")
	}

	k8sTestDataMap[c.name] = k8sTestData
}

func (c *k8sCluster) fakeIngresses() []interface{} {
	fakeIngresses := k8sTestDataMap[c.name].ingressList.Items
	ingressObjects := make([]interface{}, 0, len(fakeIngresses))
	for i := 0; i < len(fakeIngresses); i++ {
		ingressObjects = append(ingressObjects, &fakeIngresses[i])
	}
	return ingressObjects
}

func (c *k8sCluster) fakeServices() []interface{} {
	fakeServices := k8sTestDataMap[c.name].serviceList.Items
	serviceObjects := make([]interface{}, 0, len(fakeServices))
	for i := 0; i < len(fakeServices); i++ {
		serviceObjects = append(serviceObjects, &fakeServices[i])
	}
	return serviceObjects
}

func (c *k8sCluster) fakeSecrets() []interface{} {
	fakeSecrets := k8sTestDataMap[c.name].secretList.Items
	secretObjects := make([]interface{}, 0, len(fakeSecrets))
	for i := 0; i < len(fakeSecrets); i++ {
		secretObjects = append(secretObjects, &fakeSecrets[i])
	}
	return secretObjects
}

func (c *k8sCluster) fakeNodes() []interface{} {
	fakeNodes := k8sTestDataMap[c.name].nodeList.Items
	nodeObjects := make([]interface{}, 0, len(fakeNodes))
	for i := 0; i < len(fakeNodes); i++ {
		nodeObjects = append(nodeObjects, &fakeNodes[i])
	}
	return nodeObjects
}

func (c *k8sCluster) fakeSecretByKey(key string) (interface{}, bool, error) {
	for _, secret := range k8sTestDataMap[c.name].secretList.Items {
		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
		if err != nil {
			log.Fatal("Error while splittig the metanamespace key")
		}
		if secret.Namespace == namespace && secret.Name == name {
			return &secret, true, nil
		}
	}
	return nil, false, nil
}

func (c *k8sCluster) fakeServiceByKey(key string) (interface{}, bool, error) {
	for _, service := range k8sTestDataMap[c.name].serviceList.Items {
		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
		if err != nil {
			log.Fatal("Error while splittig the metanamespace key")
		}
		if service.Namespace == namespace && service.Name == name {
			return &service, true, nil
		}
	}
	return nil, false, nil
}

func (c *k8sCluster) fakeIngressKeys() []string {
	keys := []string{}
	for _, obj := range k8sTestDataMap[c.name].ingressList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}

func (c *k8sCluster) fakeServiceKeys() []string {
	keys := []string{}
	for _, obj := range k8sTestDataMap[c.name].serviceList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}

func (c *k8sCluster) fakeSecretKeys() []string {
	keys := []string{}
	for _, obj := range k8sTestDataMap[c.name].secretList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}
