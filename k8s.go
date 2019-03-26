package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/fields"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
	//watchNamespaces = "snp-hubble-dev"
	c.watchIngresses(resyncPeriod)
	c.watchServices(resyncPeriod)
	c.watchSecrets(resyncPeriod)
	c.watchNodes(resyncPeriod)
	return nil
}

func (c *k8sCluster) addK8sEventHandlers() {
	c.ingressInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedIngress,
		UpdateFunc: c.updatedIngress,
		DeleteFunc: c.deletedIngress,
	})

	c.serviceInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedService,
		UpdateFunc: c.updatedService,
		DeleteFunc: c.deletedService,
	})

	c.secretInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedSecret,
		UpdateFunc: c.updatedSecret,
		DeleteFunc: c.deletedSecret,
	})

	c.nodeInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedNode,
		DeleteFunc: c.deletedNode,
	})
}

func (c *k8sCluster) watchIngresses(resyncPeriod time.Duration) {
	lw := k8scache.NewListWatchFromClient(c.clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", watchNamespaces, fields.Everything())
	c.ingressInformer = k8scache.NewSharedInformer(lw, &extbeta1.Ingress{}, resyncPeriod)
	c.ingressCacheStore = c.ingressInformer.GetStore()
	go c.ingressInformer.Run(wait.NeverStop)
	log.Info("waiting to sync ingress for cluster: " + c.name)
	for !c.ingressInformer.HasSynced() {
	}
	c.initialIngresses = c.ingressCacheStore.ListKeys()
	log.Info(strconv.Itoa(len(c.ingressCacheStore.List())) + " ingress synced for cluster " + c.name)
}

func (c *k8sCluster) addedIngress(obj interface{}) {
	newIngress := obj.(*extbeta1.Ingress)
	var isExistingIngress bool
	for _, key := range c.initialIngresses {
		namespace, name, err := k8scache.SplitMetaNamespaceKey(key)
		if err != nil {
			log.Fatal("Error while splittig the metanamespace key")
		}
		if newIngress.Namespace == namespace && newIngress.Name == name {
			isExistingIngress = true
			break
		}
	}
	if !isExistingIngress {
		log.Info("added k8s ingress  --> " + c.name + ":" + newIngress.Namespace + ":" + newIngress.Name)
		envoyCluster.createEnvoySnapshot()
	}
}

func (c *k8sCluster) updatedIngress(oldObj interface{}, newObj interface{}) {
	oldIngress := oldObj.(*extbeta1.Ingress)
	log.Info("updated k8s ingress  --> " + c.name + ":" + oldIngress.Namespace + ":" + oldIngress.Name)
	envoyCluster.createEnvoySnapshot()
}

func (c *k8sCluster) deletedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Info("deleted k8s ingress  --> " + c.name + ":" + ingress.Namespace + ":" + ingress.Name)
	//TODO: delete from the intialIngress cache
	envoyCluster.createEnvoySnapshot()
}

func (c *k8sCluster) watchServices(resyncPeriod time.Duration) {
	lw := k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "services", watchNamespaces, fields.Everything())
	c.serviceInformer = k8scache.NewSharedInformer(lw, &v1.Service{}, resyncPeriod)
	c.serviceCacheStore = c.serviceInformer.GetStore()
	//Run the controller as a goroutine
	go c.serviceInformer.Run(wait.NeverStop)
	log.Info("waiting to sync services for cluster: " + c.name)
	for !c.serviceInformer.HasSynced() {
	}
	c.initialServices = c.serviceCacheStore.ListKeys()
	log.Info(strconv.Itoa(len(c.serviceCacheStore.List())) + " services synced for cluster " + c.name)
}

func (c *k8sCluster) addedService(obj interface{}) {
	newService := obj.(*v1.Service)
	var isExistingIngress bool
	for _, servicesKey := range c.initialServices {
		servicesKeys := strings.Split(servicesKey, "/")
		if newService.Namespace == servicesKeys[0] && newService.Name == servicesKeys[1] {
			isExistingIngress = true
			break
		}
	}
	if !isExistingIngress {
		log.Info("added k8s services  --> " + c.name + ":" + newService.Namespace + ":" + newService.Name)
		envoyCluster.createEnvoySnapshot()
	}
}

func (c *k8sCluster) updatedService(oldObj interface{}, newObj interface{}) {
	service := oldObj.(*v1.Service)
	log.Info("updated k8s services  --> " + c.name + ":" + service.Namespace + ":" + service.Name)
	envoyCluster.createEnvoySnapshot()
}

func (c *k8sCluster) deletedService(obj interface{}) {
	service := obj.(*v1.Service)
	log.Info("deleted k8s services  --> " + c.name + ":" + service.Namespace + ":" + service.Name)
	//TODO: delete from the intialServices cache
	envoyCluster.createEnvoySnapshot()
}

func (c *k8sCluster) watchSecrets(resyncPeriod time.Duration) {
	lw := k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "secrets", watchNamespaces, fields.Everything())
	c.secretInformer = k8scache.NewSharedInformer(lw, &v1.Secret{}, resyncPeriod)
	c.secretCacheStore = c.secretInformer.GetStore()
	//Run the controller as a goroutine
	go c.secretInformer.Run(wait.NeverStop)
	log.Info("waiting to sync secrets for cluster: " + c.name)
	for !c.secretInformer.HasSynced() {
	}
	c.initialSecrets = c.secretCacheStore.ListKeys()
	log.Info(strconv.Itoa(len(c.secretCacheStore.List())) + " secrets synced for cluster " + c.name)
}

func (c *k8sCluster) addedSecret(obj interface{}) {
	newSecret := obj.(*v1.Secret)
	var isExistingIngress bool
	for _, secretsKey := range c.initialSecrets {
		secretsKeys := strings.Split(secretsKey, "/")
		if newSecret.Namespace == secretsKeys[0] && newSecret.Name == secretsKeys[1] {
			isExistingIngress = true
			break
		}
	}
	if !isExistingIngress {
		log.Info("added k8s Secret  --> " + c.name + ":" + newSecret.Namespace + ":" + newSecret.Name)
		envoyCluster.createEnvoySnapshot()
	}
}

func (c *k8sCluster) updatedSecret(oldObj interface{}, newObj interface{}) {
	secret := oldObj.(*v1.Secret)
	log.Info("updated k8s Secret  --> " + c.name + ":" + secret.Namespace + ":" + secret.Name)
	envoyCluster.createEnvoySnapshot()
}

func (c *k8sCluster) deletedSecret(obj interface{}) {
	secret := obj.(*v1.Secret)
	log.Info("deleted k8s Secret  --> " + c.name + ":" + secret.Namespace + ":" + secret.Name)
	//TODO: delete from the intialServices cache
	envoyCluster.createEnvoySnapshot()
}

func (c *k8sCluster) watchNodes(resyncPeriod time.Duration) {
	lw := k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
	c.nodeInformer = k8scache.NewSharedInformer(lw, &v1.Node{}, resyncPeriod)
	c.nodeCacheStore = c.nodeInformer.GetStore()
	//Run the controller as a goroutine
	go c.nodeInformer.Run(wait.NeverStop)
	log.Info("waiting to sync nodes for cluster: " + c.name)
	for !c.nodeInformer.HasSynced() {
	}
	c.initialNodes = c.nodeCacheStore.ListKeys()
	log.Info(strconv.Itoa(len(c.nodeCacheStore.List())) + " nodes synced for cluster " + c.name)
}

func (c *k8sCluster) addedNode(obj interface{}) {
	newNode := obj.(*v1.Node)
	var isExistingNode bool
	for _, nodeKey := range c.initialNodes {
		if newNode.Name == nodeKey {
			isExistingNode = true
			break
		}
	}
	if !isExistingNode {
		log.Info("added k8s node  --> " + c.name + " " + newNode.Name)
		envoyCluster.createEnvoySnapshot()
	}
}

func (c *k8sCluster) deletedNode(obj interface{}) {
	node := obj.(*v1.Node)
	log.Info("deleted k8s node  --> " + c.name + " " + node.Name)
	//TODO: delete from the intialNodes cache
	envoyCluster.createEnvoySnapshot()
}

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
