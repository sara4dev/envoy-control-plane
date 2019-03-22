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

func TestMakeEnvoyClusters(t *testing.T) {
	setupEnvoyTest()
	envoyClustersChan := make(chan []envoycache.Resource)
	go makeEnvoyClusters(envoyClustersChan)
	envoyClusters := <-envoyClustersChan
	if len(envoyClusters) != 959 {
		t.Error("No Envoy Clusters created")
	}
}

func TestMakeEnvoyEndpoints(t *testing.T) {
	setupEnvoyTest()
	envoyEndpointsChan := make(chan []envoycache.Resource)
	go makeEnvoyEndpoints(envoyEndpointsChan)
	envoyEndpoints := <-envoyEndpointsChan
	if len(envoyEndpoints) != 959 {
		t.Error("No Envoy Endpoints created")
	}
}

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

	for _, k8sCluster := range k8sClusters {
		k8sCluster.ingressCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: func() []string {
				return []string{"test-namespace1/test-ingress1"}
			},
			ListFunc: k8sCluster.fakeIngress,
		}
		k8sCluster.serviceCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: func() []string {
				return []string{"test-namespace1/test-ingress1"}
			},
			ListFunc: k8sCluster.fakeService,
		}
	}
}

func (c *k8sCluster) fakeIngress() []interface{} {
	ingressList := v1beta1.IngressList{}
	reader, err := os.Open("test-data/" + c.name + "-ingressList.yml")
	if err != nil {
		log.Fatal("Failed to setup fake Ingress")
	}
	err = yaml.NewYAMLOrJSONDecoder(reader, 2048).Decode(&ingressList)
	if err != nil {
		log.Fatal("Failed to setup fake Ingress")
	}
	fakeIngresses := ingressList.Items
	ingressObjects := make([]interface{}, 0, len(fakeIngresses))
	for i := 0; i < len(fakeIngresses); i++ {
		ingressObjects = append(ingressObjects, &fakeIngresses[i])
	}
	return ingressObjects
}

func (c *k8sCluster) fakeService() []interface{} {
	serviceList := v1.ServiceList{}
	reader, err := os.Open("test-data/" + c.name + "-serviceList.yml")
	if err != nil {
		log.Fatal("Failed to setup fake Service")
	}
	err = yaml.NewYAMLOrJSONDecoder(reader, 2048).Decode(&serviceList)
	if err != nil {
		log.Fatal("Failed to setup fake Service")
	}
	fakeServices := serviceList.Items
	serviceObjects := make([]interface{}, 0, len(fakeServices))
	for i := 0; i < len(fakeServices); i++ {
		serviceObjects = append(serviceObjects, &fakeServices[i])
	}
	return serviceObjects
}
