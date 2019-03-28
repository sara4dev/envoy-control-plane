package main

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	k8scache "k8s.io/client-go/tools/cache"
	"log"
	"os"
	"testing"
)

type fakeSnapshotCache struct {
	envoycache.Cache
}

//func TestStartK8sControllers(t *testing.T) {
//	for _, k8sCluster := range testk8sClusters {
//		err = k8sCluster.StartK8sControllers("")
//		if err == nil {
//			t.Error("Expecting Error but got nothing")
//		}
//	}
//}

func (f *fakeSnapshotCache) SetSnapshot(node string, snapshot envoycache.Snapshot) error {
	return nil
}

// ClearSnapshot removes all status and snapshot information associated with a node.
func (f *fakeSnapshotCache) ClearSnapshot(node string) {

}

var testK8sClusters []*k8sCluster
var k8sTestDataMap1 map[string]k8sTestData
var testEnvoyCluster1 EnvoyCluster

// Test Data Setup

func setupK8sTest() {

	testK8sClusters = []*k8sCluster{
		{
			name: "cluster1",
			zone: TTC,
		},
		{
			name: "cluster2",
			zone: TTE,
		},
	}
	testEnvoyCluster1 = EnvoyCluster{}
	testEnvoyCluster1.k8sCacheStoreMap = make(map[string]*K8sCacheStore)
	testEnvoyCluster1.k8sCacheStoreMap["cluster1"] = &K8sCacheStore{
		Name:     "cluster1",
		Zone:     TTC,
		Priority: 0,
	}

	testEnvoyCluster1.k8sCacheStoreMap["cluster2"] = &K8sCacheStore{
		Name:     "cluster2",
		Zone:     TTE,
		Priority: 1,
	}

	k8sTestDataMap1 = make(map[string]k8sTestData)

	for _, testK8sCluster := range testK8sClusters {
		loadTestK8sData(testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name])
		testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].IngressCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sIngressKeys,
			ListFunc:     testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sIngresses,
			GetByKeyFunc: testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sIngressByKey,
		}
		testK8sCluster.initialIngresses = testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].IngressCacheStore.ListKeys()
		testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].ServiceCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sServiceKeys,
			ListFunc:     testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sServices,
			GetByKeyFunc: testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sServiceByKey,
		}
		testK8sCluster.initialServices = testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].ServiceCacheStore.ListKeys()
		testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].SecretCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sSecretKeys,
			ListFunc:     testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sSecrets,
			GetByKeyFunc: testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sSecretByKey,
		}
		testK8sCluster.initialSecrets = testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].SecretCacheStore.ListKeys()
		testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].NodeCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sNodeKeys,
			ListFunc:     testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sNodes,
			GetByKeyFunc: testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].fakeK8sNodeByKey,
		}
		testK8sCluster.initialNodes = testEnvoyCluster1.k8sCacheStoreMap[testK8sCluster.name].NodeCacheStore.ListKeys()
	}
}

func loadTestK8sData(k8sCacheStore *K8sCacheStore) {
	k8sTestData := k8sTestData{}

	//load fake ingresses
	loadObjFromFile("test-data/"+k8sCacheStore.Name+"-ingressList.yml", &k8sTestData.ingressList)

	//load fake services
	loadObjFromFile("test-data/"+k8sCacheStore.Name+"-serviceList.yml", &k8sTestData.serviceList)

	//load fake secrets
	loadObjFromFile("test-data/"+k8sCacheStore.Name+"-secretList.yml", &k8sTestData.secretList)

	//load fake nodes
	loadObjFromFile("test-data/"+k8sCacheStore.Name+"-nodeList.yml", &k8sTestData.nodeList)

	k8sTestDataMap1[k8sCacheStore.Name] = k8sTestData
}

func (c *K8sCacheStore) fakeK8sIngresses() []interface{} {
	fakeIngresses := k8sTestDataMap1[c.Name].ingressList.Items
	ingressObjects := make([]interface{}, 0, len(fakeIngresses))
	for i := 0; i < len(fakeIngresses); i++ {
		ingressObjects = append(ingressObjects, &fakeIngresses[i])
	}
	return ingressObjects
}

func (c *K8sCacheStore) fakeK8sServices() []interface{} {
	fakeServices := k8sTestDataMap1[c.Name].serviceList.Items
	serviceObjects := make([]interface{}, 0, len(fakeServices))
	for i := 0; i < len(fakeServices); i++ {
		serviceObjects = append(serviceObjects, &fakeServices[i])
	}
	return serviceObjects
}

func (c *K8sCacheStore) fakeK8sSecrets() []interface{} {
	fakeSecrets := k8sTestDataMap1[c.Name].secretList.Items
	secretObjects := make([]interface{}, 0, len(fakeSecrets))
	for i := 0; i < len(fakeSecrets); i++ {
		secretObjects = append(secretObjects, &fakeSecrets[i])
	}
	return secretObjects
}

func (c *K8sCacheStore) fakeK8sNodes() []interface{} {
	fakeNodes := k8sTestDataMap1[c.Name].nodeList.Items
	nodeObjects := make([]interface{}, 0, len(fakeNodes))
	for i := 0; i < len(fakeNodes); i++ {
		nodeObjects = append(nodeObjects, &fakeNodes[i])
	}
	return nodeObjects
}

func (c *K8sCacheStore) fakeK8sIngressByKey(key string) (interface{}, bool, error) {
	for _, ingress := range k8sTestDataMap1[c.Name].ingressList.Items {
		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
		if err != nil {
			log.Fatal("Error while splittig the metanamespace key")
		}
		if ingress.Namespace == namespace && ingress.Name == name {
			return &ingress, true, nil
		}
	}
	return nil, false, nil
}

func (c *K8sCacheStore) fakeK8sSecretByKey(key string) (interface{}, bool, error) {
	for _, secret := range k8sTestDataMap1[c.Name].secretList.Items {
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

func (c *K8sCacheStore) fakeK8sServiceByKey(key string) (interface{}, bool, error) {
	for _, service := range k8sTestDataMap1[c.Name].serviceList.Items {
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

func (c *K8sCacheStore) fakeK8sNodeByKey(key string) (interface{}, bool, error) {
	for _, node := range k8sTestDataMap1[c.Name].nodeList.Items {
		if node.Name == key {
			return &node, true, nil
		}
	}
	return nil, false, nil
}

func (c *K8sCacheStore) fakeK8sIngressKeys() []string {
	keys := []string{}
	for _, obj := range k8sTestDataMap1[c.Name].ingressList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}

func (c *K8sCacheStore) fakeK8sServiceKeys() []string {
	keys := []string{}
	for _, obj := range k8sTestDataMap1[c.Name].serviceList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}

func (c *K8sCacheStore) fakeK8sSecretKeys() []string {
	keys := []string{}
	for _, obj := range k8sTestDataMap1[c.Name].secretList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}

func (c *K8sCacheStore) fakeK8sNodeKeys() []string {
	keys := []string{}
	for _, obj := range k8sTestDataMap1[c.Name].nodeList.Items {
		keys = append(keys, obj.Name)
	}
	return keys
}

func TestAddedIngress_new_ingress_added(t *testing.T) {
	tested := false
	setupK8sTest()
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	var i int32 = 1
	for _, k8sCluster := range testK8sClusters {
		tested = true
		newIngress := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-newIngress.yml", &newIngress)
		k8sCluster.addedObj(&newIngress)
		//Check if the Envoy Snapshot is called for new ingress
		if envoyCluster.version != i {
			t.Error("Envoy Snapshot is not called")
		}
		i++
	}

	if !tested {
		t.Error("No tests ran")
	}
}

func TestAddedIngress_initial_ingress_added(t *testing.T) {
	tested := false
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	for _, k8sCluster := range testK8sClusters {
		tested = true
		newIngress := extbeta1.Ingress{}
		loadObjFromFile("test-data/"+k8sCluster.name+"-initialIngress.yml", &newIngress)
		k8sCluster.addedObj(&newIngress)
		//Check if the Envoy Snapshot is NOT called for initial existing ingress
		if envoyCluster.version != 0 {
			t.Error("Envoy Snapshot should not be called")
		}
	}

	if !tested {
		t.Error("No tests ran")
	}
}

func TestUpdatedIngress_new_ingress_updated(t *testing.T) {
	tested := false
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	var i int32 = 1
	for _, k8sCluster := range testK8sClusters {
		tested = true
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

	if !tested {
		t.Error("No tests ran")
	}
}

func TestUpdatedIngress_new_ingress_status_updated(t *testing.T) {
	tested := false
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	for _, k8sCluster := range testK8sClusters {
		tested = true
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

	if !tested {
		t.Error("No tests ran")
	}
}

func TestDeletedIngress_delete_initial_ingress(t *testing.T) {
	tested := false
	envoyCluster = EnvoyCluster{
		envoySnapshotCache: &fakeSnapshotCache{},
	}
	var i int32 = 1
	for _, k8sCluster := range testK8sClusters {
		tested = true
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

	if !tested {
		t.Error("No tests ran")
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
