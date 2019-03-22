package main

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
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
	}
}

func (c *k8sCluster) fakeIngress() []interface{} {
	ingressList := v1beta1.IngressList{}
	reader, err := os.Open("test-data/" + c.name + "-ingressList.yml")
	if err != nil {
		log.Fatal("Failed to setup test data")
	}
	err = yaml.NewYAMLOrJSONDecoder(reader, 100).Decode(&ingressList)
	if err != nil {
		log.Fatal("Failed to setup test data")
	}
	fakeIngresses := ingressList.Items
	if err != nil {
		log.Fatal("Failed to setup test data")
	}
	ingressObjects := make([]interface{}, 0, len(fakeIngresses))
	for i := 0; i < len(fakeIngresses); i++ {
		ingressObjects = append(ingressObjects, &fakeIngresses[i])
	}
	return ingressObjects
}
