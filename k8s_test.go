package main

import (
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"k8s.io/api/core/v1"
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

type k8sTestData_1 struct {
	clusterName string
	ingressList extbeta1.IngressList
	serviceList v1.ServiceList
	secretList  v1.SecretList
	nodeList    v1.NodeList
}

var k8sTestDataMap_1 map[string]*k8sTestData_1
var testEnvoyCluster_1 EnvoyCluster
var testK8sClusters []*k8sCluster

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
	testEnvoyCluster_1 = EnvoyCluster{}
	testEnvoyCluster_1.k8sCacheStoreMap = make(map[string]*K8sCacheStore)
	testEnvoyCluster_1.k8sCacheStoreMap["cluster1"] = &K8sCacheStore{
		Name:     "cluster1",
		Zone:     TTC,
		Priority: 0,
	}

	testEnvoyCluster_1.k8sCacheStoreMap["cluster2"] = &K8sCacheStore{
		Name:     "cluster2",
		Zone:     TTE,
		Priority: 1,
	}

	k8sTestDataMap_1 = make(map[string]*k8sTestData_1)

	for _, k8sCacheStore := range testEnvoyCluster_1.k8sCacheStoreMap {
		k8sTestDataMap_1[k8sCacheStore.Name] = &k8sTestData_1{
			clusterName: k8sCacheStore.Name,
		}
	}

	for _, testK8sCluster := range testK8sClusters {
		k8sCacheStore := testEnvoyCluster_1.k8sCacheStoreMap[testK8sCluster.name]
		k8sTestData_1 := k8sTestDataMap_1[testK8sCluster.name]
		loadTestK8sData(k8sTestData_1)
		k8sCacheStore.IngressCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: k8sTestData_1.fakeK8sIngressKeys,
			ListFunc:     k8sTestData_1.fakeK8sIngresses,
			GetByKeyFunc: k8sTestData_1.fakeK8sIngressByKey,
		}
		testK8sCluster.initialIngresses = k8sCacheStore.IngressCacheStore.ListKeys()
		k8sCacheStore.ServiceCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: k8sTestData_1.fakeK8sServiceKeys,
			ListFunc:     k8sTestData_1.fakeK8sServices,
			GetByKeyFunc: k8sTestData_1.fakeK8sServiceByKey,
		}
		testK8sCluster.initialServices = k8sCacheStore.ServiceCacheStore.ListKeys()
		k8sCacheStore.SecretCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: k8sTestData_1.fakeK8sSecretKeys,
			ListFunc:     k8sTestData_1.fakeK8sSecrets,
			GetByKeyFunc: k8sTestData_1.fakeK8sSecretByKey,
		}
		testK8sCluster.initialSecrets = k8sCacheStore.SecretCacheStore.ListKeys()
		k8sCacheStore.NodeCacheStore = &k8scache.FakeCustomStore{
			ListKeysFunc: k8sTestData_1.fakeK8sNodeKeys,
			ListFunc:     k8sTestData_1.fakeK8sNodes,
			GetByKeyFunc: k8sTestData_1.fakeK8sNodeByKey,
		}
		testK8sCluster.initialNodes = k8sCacheStore.NodeCacheStore.ListKeys()
	}
}

func loadTestK8sData(k8sTestData_1 *k8sTestData_1) {
	//load fake ingresses
	loadObjFromFile("test-data/"+k8sTestData_1.clusterName+"-ingressList.yml", &k8sTestData_1.ingressList)

	//load fake services
	loadObjFromFile("test-data/"+k8sTestData_1.clusterName+"-serviceList.yml", &k8sTestData_1.serviceList)

	//load fake secrets
	loadObjFromFile("test-data/"+k8sTestData_1.clusterName+"-secretList.yml", &k8sTestData_1.secretList)

	//load fake nodes
	loadObjFromFile("test-data/"+k8sTestData_1.clusterName+"-nodeList.yml", &k8sTestData_1.nodeList)
}

func (c *k8sTestData_1) fakeK8sIngresses() []interface{} {
	fakeIngresses := c.ingressList.Items
	ingressObjects := make([]interface{}, 0, len(fakeIngresses))
	for i := 0; i < len(fakeIngresses); i++ {
		ingressObjects = append(ingressObjects, &fakeIngresses[i])
	}
	return ingressObjects
}

func (c *k8sTestData_1) fakeK8sServices() []interface{} {
	fakeServices := c.serviceList.Items
	serviceObjects := make([]interface{}, 0, len(fakeServices))
	for i := 0; i < len(fakeServices); i++ {
		serviceObjects = append(serviceObjects, &fakeServices[i])
	}
	return serviceObjects
}

func (c *k8sTestData_1) fakeK8sSecrets() []interface{} {
	fakeSecrets := c.secretList.Items
	secretObjects := make([]interface{}, 0, len(fakeSecrets))
	for i := 0; i < len(fakeSecrets); i++ {
		secretObjects = append(secretObjects, &fakeSecrets[i])
	}
	return secretObjects
}

func (c *k8sTestData_1) fakeK8sNodes() []interface{} {
	fakeNodes := c.nodeList.Items
	nodeObjects := make([]interface{}, 0, len(fakeNodes))
	for i := 0; i < len(fakeNodes); i++ {
		nodeObjects = append(nodeObjects, &fakeNodes[i])
	}
	return nodeObjects
}

func (c *k8sTestData_1) fakeK8sIngressByKey(key string) (interface{}, bool, error) {
	for _, ingress := range c.ingressList.Items {
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

func (c *k8sTestData_1) fakeK8sSecretByKey(key string) (interface{}, bool, error) {
	for _, secret := range c.secretList.Items {
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

func (c *k8sTestData_1) fakeK8sServiceByKey(key string) (interface{}, bool, error) {
	for _, service := range c.serviceList.Items {
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

func (c *k8sTestData_1) fakeK8sNodeByKey(key string) (interface{}, bool, error) {
	for _, node := range c.nodeList.Items {
		if node.Name == key {
			return &node, true, nil
		}
	}
	return nil, false, nil
}

func (c *k8sTestData_1) fakeK8sIngressKeys() []string {
	keys := []string{}
	for _, obj := range c.ingressList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}

func (c *k8sTestData_1) fakeK8sServiceKeys() []string {
	keys := []string{}
	for _, obj := range c.serviceList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}

func (c *k8sTestData_1) fakeK8sSecretKeys() []string {
	keys := []string{}
	for _, obj := range c.secretList.Items {
		keys = append(keys, obj.Namespace+"/"+obj.Name)
	}
	return keys
}

func (c *k8sTestData_1) fakeK8sNodeKeys() []string {
	keys := []string{}
	for _, obj := range c.nodeList.Items {
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
