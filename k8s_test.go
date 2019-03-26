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

//func init() {
//	k8sClusters = []*k8sCluster{
//		{
//			name: "cluster1",
//			zone: TTC,
//		},
//		{
//			name: "cluster2",
//			zone: TTE,
//		},
//	}
//}

func TestStartK8sControllers(t *testing.T) {
	for _, k8sCluster := range k8sClusters {
		err = k8sCluster.startK8sControllers("")
		if err == nil {
			t.Error("Expecting Error but got nothing")
		}
	}
}

//func TestWatchIngresses(t *testing.T) {
//	for _, k8sCluster := range k8sClusters {
//		k8sCluster.clientSet = fake.NewSimpleClientset(&extbeta1.Ingress{})
//		//k8sCluster.clientSet.(*fake.Clientset).Fake.Resources =
//		//fakeIngress := k8sCluster.clientSet.ExtensionsV1beta1().RESTClient().Get()
//		//log.Info(fakeIngress)
//		//source := fcache.NewFakeControllerSource()
//		//k8sCluster.ingressInformer = k8scache.NewSharedInformer(source, &extbeta1.Ingress{}, 1*time.Second)
//		//source.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}})
//		//source.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod2"}})
//		//
//		//// create the shared informer and resync every 1s
//		//informer := NewSharedInformer(source, &v1.Pod{}, 1*time.Second).(*sharedIndexInformer)
//		k8sCluster.watchIngresses(time.Second * 1)
//	}
//}

//func TestAddK8sEventHandlers(t *testing.T) {
//	for _, k8sCluster := range k8sClusters {
//		k8sCluster.addK8sEventHandlers()
//	}
//}

type fakeSnapshotCache struct {
	envoycache.Cache
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
		k8sCluster.addedIngress(&newIngress)
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
		k8sCluster.addedIngress(&newIngress)
		//Check if the Envoy Snapshot is NOT called for initial existing ingress
		if envoyCluster.version != 0 {
			t.Error("Envoy Snapshot should not be called")
		}
	}
}

func TestAddedIngress_new_ingress_updated(t *testing.T) {
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	var i int32 = 1
	for _, k8sCluster := range k8sClusters {
		oldObj := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-newIngress.yml", &oldObj)

		newObj := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-newIngress.yml", &newObj)

		k8sCluster.updatedIngress(&oldObj, &newObj)
		//Check if the Envoy Snapshot is called for updated ingress
		if envoyCluster.version != i {
			t.Error("Envoy Snapshot should not be called")
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
