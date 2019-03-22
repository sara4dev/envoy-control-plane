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
	if len(envoyClusters) != 833 {
		t.Error("No Envoy Clusters created")
	}
}

func setupEnvoyTest() {
	ttcIngressCacheStore := &k8scache.FakeCustomStore{
		ListKeysFunc: func() []string {
			return []string{"test-namespace1/test-ingress1"}
		},
		ListFunc: func() []interface{} {
			fakeIngresses, err := fakeIngress()
			if err != nil {
				log.Fatal("Failed to setup test data")
			}
			ingressList := make([]interface{}, 0, len(fakeIngresses))
			for i := 0; i < len(fakeIngresses); i++ {
				ingressList = append(ingressList, &fakeIngresses[i])
			}
			return ingressList
		},
	}
	//tteIngressCacheStore := &k8scache.FakeCustomStore{
	//	ListKeysFunc: func() []string {
	//		return []string{"test-namespace1/test-ingress1"}
	//	},
	//}
	k8sClusters = []*k8sCluster{
		{
			name:              "test-ttc",
			zone:              TTC,
			ingressCacheStore: ttcIngressCacheStore,
		},
		//{
		//	name: "test-tte",
		//	zone: TTE,
		//	ingressCacheStore: tteIngressCacheStore,
		//},
	}
}

func fakeIngress() ([]v1beta1.Ingress, error) {
	ingressList := v1beta1.IngressList{}
	reader, err := os.Open("test-data/ingressList.yml")
	if err != nil {
		return nil, err
	}
	err = yaml.NewYAMLOrJSONDecoder(reader, 100).Decode(&ingressList)
	if err != nil {
		return nil, err
	}
	return ingressList.Items, nil
}
