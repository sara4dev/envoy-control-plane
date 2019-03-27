package main

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"time"
)

type zone int

//TODO remove hardcoded zone
const (
	TTC zone = 0
	TTE zone = 1
)

type k8sCluster struct {
	name              string
	zone              zone
	priority          uint32
	clientSet         kubernetes.Interface
	ingressInformer   k8scache.SharedInformer
	ingressCacheStore k8scache.Store
	initialIngresses  []string
	serviceInformer   k8scache.SharedInformer
	serviceCacheStore k8scache.Store
	initialServices   []string
	secretInformer    k8scache.SharedInformer
	secretCacheStore  k8scache.Store
	initialSecrets    []string
	nodeInformer      k8scache.SharedInformer
	nodeCacheStore    k8scache.Store
	initialNodes      []string
}

var (
	k8sClusters     []*k8sCluster
	resyncPeriod    time.Duration
	signal          chan struct{}
	err             error
	watchNamespaces string
)

func (c *k8sCluster) startK8sControllers(kubeConfigPath string) error {
	c.clientSet, err = newKubeClient(kubeConfigPath, c.name)
	if err != nil {
		return err
	}
	watchNamespaces = v1.NamespaceAll
	//watchNamespaces = "kube-system"
	c.watchObjects(resyncPeriod, &extbeta1.Ingress{})
	c.watchObjects(resyncPeriod, &v1.Service{})
	c.watchObjects(resyncPeriod, &v1.Secret{})
	c.watchObjects(resyncPeriod, &v1.Node{})
	return nil
}

func (c *k8sCluster) addK8sEventHandlers() {
	c.ingressInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedObj,
		UpdateFunc: c.updatedObj,
		DeleteFunc: c.deletedObj,
	})

	c.serviceInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedObj,
		UpdateFunc: c.updatedObj,
		DeleteFunc: c.deletedObj,
	})

	c.secretInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedObj,
		UpdateFunc: c.updatedObj,
		DeleteFunc: c.deletedObj,
	})

	c.nodeInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedObj,
		DeleteFunc: c.deletedObj,
	})
}

func (c *k8sCluster) watchObjects(resyncPeriod time.Duration, objType runtime.Object) {
	var lw *k8scache.ListWatch
	var informer *k8scache.SharedInformer
	var cacheStore *k8scache.Store
	var initialObjects *[]string
	switch objType.(type) {
	case *extbeta1.Ingress:
		lw = k8scache.NewListWatchFromClient(c.clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", watchNamespaces, fields.Everything())
		informer = &c.ingressInformer
		cacheStore = &c.ingressCacheStore
		initialObjects = &c.initialIngresses
	case *v1.Service:
		lw = k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "services", watchNamespaces, fields.Everything())
		informer = &c.serviceInformer
		cacheStore = &c.serviceCacheStore
		initialObjects = &c.initialServices
	case *v1.Secret:
		lw = k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "secrets", watchNamespaces, fields.Everything())
		informer = &c.secretInformer
		cacheStore = &c.secretCacheStore
		initialObjects = &c.initialSecrets
	case *v1.Node:
		lw = k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
		informer = &c.nodeInformer
		cacheStore = &c.nodeCacheStore
		initialObjects = &c.initialNodes
	}

	*informer = k8scache.NewSharedInformer(lw, objType, resyncPeriod)
	*cacheStore = (*informer).GetStore()
	go (*informer).Run(wait.NeverStop)
	log.Infof("waiting to sync ingress for cluster: %v", c.name)
	for !(*informer).HasSynced() {
	}
	*initialObjects = (*cacheStore).ListKeys()
	log.Infof("%v %T synced for cluster %v", len((*cacheStore).List()), objType, c.name)
}

func (c *k8sCluster) addedObj(obj interface{}) {
	var objNamespace string
	var objName string
	var initialObjects *[]string

	switch newObj := obj.(type) {
	case *extbeta1.Ingress:
		objNamespace = newObj.Namespace
		objName = newObj.Name
		initialObjects = &c.initialIngresses
	case *v1.Service:
		objNamespace = newObj.Namespace
		objName = newObj.Name
		initialObjects = &c.initialServices
	case *v1.Secret:
		objNamespace = newObj.Namespace
		objName = newObj.Name
		initialObjects = &c.initialSecrets
		if newObj.Data["tls.crt"] == nil || newObj.Data["tls.key"] == nil {
			log.Infof("skipping envoy updates, as it's not a TLS secret --> %v:%v:%v", c.name, objNamespace, objName)
			return
		}
	case *v1.Node:
		objNamespace = newObj.Namespace
		objName = newObj.Name
		initialObjects = &c.initialNodes
	}

	for _, key := range *initialObjects {
		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
		if err != nil {
			log.Fatalf("Error while splitting the metanamespacekey %v in the cluster %v", key, c.name)
		}
		if objNamespace == namespace && objName == name {
			log.Infof("skipping envoy updates, as it's an existing %T --> %v:%v:%v", obj, c.name, objNamespace, objName)
			return
		}
	}

	log.Infof("added k8s %T  --> %v:%v:%v", obj, c.name, objNamespace, objName)
	envoyCluster.createEnvoySnapshot()
}

func (c *k8sCluster) updatedObj(oldObj interface{}, newObj interface{}) {
	var objNamespace string
	var objName string

	switch updatedObj := oldObj.(type) {
	case *extbeta1.Ingress:
		objNamespace = updatedObj.Namespace
		objName = updatedObj.Name
	case *v1.Service:
		objNamespace = updatedObj.Namespace
		objName = updatedObj.Name
	case *v1.Secret:
		objNamespace = updatedObj.Namespace
		objName = updatedObj.Name
		if updatedObj.Data["tls.crt"] == nil || updatedObj.Data["tls.key"] == nil {
			log.Infof("skipping envoy updates, as it's not a TLS secret --> %v:%v:%v", c.name, objNamespace, objName)
			return
		}
	case *v1.Node:
		objNamespace = updatedObj.Namespace
		objName = updatedObj.Name
	}

	if cmp.Equal(oldObj, newObj,
		cmpopts.IgnoreFields(extbeta1.Ingress{}, "Status"),
		cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")) {
		log.Infof("skipping envoy updates, as only %T status has changed --> %v:%v:%v", oldObj, c.name, objNamespace, objName)
		return
	}
	log.Infof("updated k8s %T --> %v:%v:%v", oldObj, c.name, objNamespace, objName)
	envoyCluster.createEnvoySnapshot()
}

func (c *k8sCluster) deletedObj(obj interface{}) {
	var objNamespace string
	var objName string
	var initialObjects *[]string

	switch delObj := obj.(type) {
	case *extbeta1.Ingress:
		objNamespace = delObj.Namespace
		objName = delObj.Name
		initialObjects = &c.initialIngresses
	case *v1.Service:
		objNamespace = delObj.Namespace
		objName = delObj.Name
		initialObjects = &c.initialServices
	case *v1.Secret:
		objNamespace = delObj.Namespace
		objName = delObj.Name
		initialObjects = &c.initialSecrets
		if delObj.Data["tls.crt"] == nil || delObj.Data["tls.key"] == nil {
			log.Infof("skipping envoy updates, as it's not a TLS secret --> %v:%v:%v", c.name, objNamespace, objName)
			return
		}
	case *v1.Node:
		objNamespace = delObj.Namespace
		objName = delObj.Name
		initialObjects = &c.initialNodes
	}

	log.Infof("deleted k8s %T  --> %v:%v:%v", obj, c.name, objNamespace, objName)
	var index int
	var key string
	for index, key = range *initialObjects {
		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
		if err != nil {
			log.Fatal("Error while splittig the metanamespace key")
		}
		if objNamespace == namespace && objName == name {
			//delete from the intialObjects cache
			*initialObjects = append((*initialObjects)[:index], (*initialObjects)[index+1:]...)
			break
		}
	}
	envoyCluster.createEnvoySnapshot()
}

//func (c *k8sCluster) watchServices(resyncPeriod time.Duration) {
//	lw := k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "services", watchNamespaces, fields.Everything())
//	c.serviceInformer = k8scache.NewSharedInformer(lw, &v1.Service{}, resyncPeriod)
//	c.serviceCacheStore = c.serviceInformer.GetStore()
//	//Run the controller as a goroutine
//	go c.serviceInformer.Run(wait.NeverStop)
//	log.Info("waiting to sync services for cluster: " + c.name)
//	for !c.serviceInformer.HasSynced() {
//	}
//	c.initialServices = c.serviceCacheStore.ListKeys()
//	log.Info(strconv.Itoa(len(c.serviceCacheStore.List())) + " services synced for cluster " + c.name)
//}
//
//func (c *k8sCluster) addedService(obj interface{}) {
//	newService := obj.(*v1.Service)
//	var isExistingIngress bool
//	for _, servicesKey := range c.initialServices {
//		servicesKeys := strings.Split(servicesKey, "/")
//		if newService.Namespace == servicesKeys[0] && newService.Name == servicesKeys[1] {
//			isExistingIngress = true
//			break
//		}
//	}
//	if !isExistingIngress {
//		log.Info("added k8s services  --> " + c.name + ":" + newService.Namespace + ":" + newService.Name)
//		envoyCluster.createEnvoySnapshot()
//	}
//}
//
//func (c *k8sCluster) updatedService(oldObj interface{}, newObj interface{}) {
//	service := oldObj.(*v1.Service)
//	log.Info("updated k8s services  --> " + c.name + ":" + service.Namespace + ":" + service.Name)
//	envoyCluster.createEnvoySnapshot()
//}
//
//func (c *k8sCluster) deletedService(obj interface{}) {
//	service := obj.(*v1.Service)
//	log.Info("deleted k8s services  --> " + c.name + ":" + service.Namespace + ":" + service.Name)
//	//TODO: delete from the intialServices cache
//	envoyCluster.createEnvoySnapshot()
//}
//
//func (c *k8sCluster) watchSecrets(resyncPeriod time.Duration) {
//	lw := k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "secrets", watchNamespaces, fields.Everything())
//	c.secretInformer = k8scache.NewSharedInformer(lw, &v1.Secret{}, resyncPeriod)
//	c.secretCacheStore = c.secretInformer.GetStore()
//	//Run the controller as a goroutine
//	go c.secretInformer.Run(wait.NeverStop)
//	log.Info("waiting to sync secrets for cluster: " + c.name)
//	for !c.secretInformer.HasSynced() {
//	}
//	c.initialSecrets = c.secretCacheStore.ListKeys()
//	log.Info(strconv.Itoa(len(c.secretCacheStore.List())) + " secrets synced for cluster " + c.name)
//}
//
//func (c *k8sCluster) addedSecret(obj interface{}) {
//	newSecret := obj.(*v1.Secret)
//	var isExistingIngress bool
//	for _, secretsKey := range c.initialSecrets {
//		secretsKeys := strings.Split(secretsKey, "/")
//		if newSecret.Namespace == secretsKeys[0] && newSecret.Name == secretsKeys[1] {
//			isExistingIngress = true
//			break
//		}
//	}
//	if !isExistingIngress {
//		log.Info("added k8s Secret  --> " + c.name + ":" + newSecret.Namespace + ":" + newSecret.Name)
//		envoyCluster.createEnvoySnapshot()
//	}
//}
//
//func (c *k8sCluster) updatedSecret(oldObj interface{}, newObj interface{}) {
//	secret := oldObj.(*v1.Secret)
//	log.Info("updated k8s Secret  --> " + c.name + ":" + secret.Namespace + ":" + secret.Name)
//	envoyCluster.createEnvoySnapshot()
//}
//
//func (c *k8sCluster) deletedSecret(obj interface{}) {
//	secret := obj.(*v1.Secret)
//	log.Info("deleted k8s Secret  --> " + c.name + ":" + secret.Namespace + ":" + secret.Name)
//	//TODO: delete from the intialServices cache
//	envoyCluster.createEnvoySnapshot()
//}
//
//func (c *k8sCluster) watchNodes(resyncPeriod time.Duration) {
//	lw := k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
//	c.nodeInformer = k8scache.NewSharedInformer(lw, &v1.Node{}, resyncPeriod)
//	c.nodeCacheStore = c.nodeInformer.GetStore()
//	//Run the controller as a goroutine
//	go c.nodeInformer.Run(wait.NeverStop)
//	log.Info("waiting to sync nodes for cluster: " + c.name)
//	for !c.nodeInformer.HasSynced() {
//	}
//	c.initialNodes = c.nodeCacheStore.ListKeys()
//	log.Info(strconv.Itoa(len(c.nodeCacheStore.List())) + " nodes synced for cluster " + c.name)
//}
//
//func (c *k8sCluster) addedNode(obj interface{}) {
//	newNode := obj.(*v1.Node)
//	var isExistingNode bool
//	for _, nodeKey := range c.initialNodes {
//		if newNode.Name == nodeKey {
//			isExistingNode = true
//			break
//		}
//	}
//	if !isExistingNode {
//		log.Info("added k8s node  --> " + c.name + " " + newNode.Name)
//		envoyCluster.createEnvoySnapshot()
//	}
//}
//
//func (c *k8sCluster) deletedNode(obj interface{}) {
//	node := obj.(*v1.Node)
//	log.Info("deleted k8s node  --> " + c.name + " " + node.Name)
//	//TODO: delete from the intialNodes cache
//	envoyCluster.createEnvoySnapshot()
//}

// NewKubeClient k8s client.
func newKubeClient(kubeconfigPath string, context string) (kubernetes.Interface, error) {
	var config *restclient.Config
	apiConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("error creating kube config from file: %s", err)
	}
	authInfo := apiConfig.Contexts[context].AuthInfo
	token := apiConfig.AuthInfos[authInfo].Token
	cluster := apiConfig.Contexts[context].Cluster
	server := apiConfig.Clusters[cluster].Server
	config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{AuthInfo: clientcmdapi.AuthInfo{Token: token}, ClusterInfo: clientcmdapi.Cluster{Server: server}}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating client config: %s", err)
	}

	// Use protobufs for communication with apiserver.
	config.ContentType = "application/vnd.kubernetes.protobuf"

	return kubernetes.NewForConfig(config)
}
