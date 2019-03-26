package main

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"log"
	"os"
	"testing"
)

type fakeSnapshotCache struct {
	envoycache.Cache
}

func TestStartK8sControllers(t *testing.T) {
	for _, k8sCluster := range k8sClusters {
		err = k8sCluster.startK8sControllers("")
		if err == nil {
			t.Error("Expecting Error but got nothing")
		}
	}
}

func (f *fakeSnapshotCache) SetSnapshot(node string, snapshot envoycache.Snapshot) error {
	return nil
}

// ClearSnapshot removes all status and snapshot information associated with a node.
func (f *fakeSnapshotCache) ClearSnapshot(node string) {

}

func TestAddedIngress_new_ingress_added(t *testing.T) {
	setupEnvoyTest()
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	var i int32 = 1
	for _, k8sCluster := range k8sClusters {
		newIngress := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-newIngress.yml", &newIngress)
		k8sCluster.addedObj(&newIngress)
		//Check if the Envoy Snapshot is called for new ingress
		if envoyCluster.version != i {
			t.Error("Envoy Snapshot is not called")
		}
		i++
	}
}

func TestAddedIngress_initial_ingress_added(t *testing.T) {
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	for _, k8sCluster := range k8sClusters {
		newIngress := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-initialIngress.yml", &newIngress)
		k8sCluster.addedObj(&newIngress)
		//Check if the Envoy Snapshot is NOT called for initial existing ingress
		if envoyCluster.version != 0 {
			t.Error("Envoy Snapshot should not be called")
		}
	}
}

func TestUpdatedIngress_new_ingress_updated(t *testing.T) {
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	var i int32 = 1
	for _, k8sCluster := range k8sClusters {
		oldObj := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-newIngress.yml", &oldObj)

		newObj := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-newIngress-updated.yml", &newObj)

		k8sCluster.updatedObj(&oldObj, &newObj)
		//Check if the Envoy Snapshot is called for updated ingress
		if envoyCluster.version != i {
			t.Error("Envoy Snapshot is not called")
		}
		i++
	}
}

func TestUpdatedIngress_new_ingress_status_updated(t *testing.T) {
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	for _, k8sCluster := range k8sClusters {
		oldObj := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-newIngress.yml", &oldObj)

		newObj := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-newIngress-statusUpdated.yml", &newObj)

		k8sCluster.updatedObj(&oldObj, &newObj)
		//Check if the Envoy Snapshot is NOT called for status updates
		if envoyCluster.version != 0 {
			t.Error("Envoy Snapshot should not be called")
		}
	}
}

func TestDeletedIngress_delete_initial_ingress(t *testing.T) {
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	var i int32 = 1
	for _, k8sCluster := range k8sClusters {
		initialIngressCount := len(k8sCluster.initialIngresses)
		delObj := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-initialIngress.yml", &delObj)

		k8sCluster.deletedObj(&delObj)
		//Check if the Envoy Snapshot is NOT called for status updates
		if envoyCluster.version != i {
			t.Error("Envoy Snapshot is not called")
		}
		if len(k8sCluster.initialIngresses) != initialIngressCount-1 {
			t.Error("Ingress is not deleted from InitialIngress cache")
		}
		i++
	}
}

func loadObjFromFile(fileName string, obj runtime.Object) {
	reader, err := os.Open(fileName)
	if err != nil {
		log.Fatal("Failed to open file " + fileName)
	}
	err = yaml.NewYAMLOrJSONDecoder(reader, 2048).Decode(&obj)
	if err != nil {
		log.Fatal("Failed to parse obj: " + obj.GetObjectKind().GroupVersionKind().Kind)
	}
}
