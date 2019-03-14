package main

import (
	"fmt"
	"github.com/urfave/cli"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type zone int

const (
	TTC zone = 0
	TTE zone = 1
)

type k8sCluster struct {
	name              string
	zone              zone
	priority          uint32
	clientSet         kubernetes.Interface
	ingressInformer   k8scache.SharedIndexInformer
	ingressCacheStore k8scache.Store
	initialIngresses  []string
	serviceInformer   k8scache.SharedIndexInformer
	serviceCacheStore k8scache.Store
	initialServices   []string
	nodeInformer      k8scache.SharedIndexInformer
	nodeCacheStore    k8scache.Store
	initialNodes      []string
}

var (
	k8sClusters  []*k8sCluster
	resyncPeriod time.Duration
	signal       chan struct{}
	err          error
)

func (c *k8sCluster) startK8sControllers(ctx *cli.Context) {
	//k8sSyncController := k8sController{clusterName:k8sCluster}
	c.clientSet, err = newKubeClient(ctx.String("kube-config"), c.name)
	if err != nil {
		log.Fatalf("error newKubeClient: %s", err.Error())
	}

	informerFactory := informers.NewSharedInformerFactory(c.clientSet, resyncPeriod)

	c.watchIngresses(informerFactory, resyncPeriod)
	c.watchNodes(informerFactory, resyncPeriod)
	c.watchServices(informerFactory, resyncPeriod)
}

func (c *k8sCluster) addK8sEventHandlers() {
	c.ingressInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedIngress,
		UpdateFunc: c.updatedIngress,
		DeleteFunc: c.deletedIngress,
	})

	c.nodeInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedNode,
		DeleteFunc: c.deletedNode,
	})

	c.serviceInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedService,
		UpdateFunc: c.updatedService,
		DeleteFunc: c.deletedService,
	})
}

func (c *k8sCluster) watchIngresses(informerFactory informers.SharedInformerFactory, resyncPeriod time.Duration) {
	c.ingressInformer = informerFactory.Extensions().V1beta1().Ingresses().Informer()
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
		createEnvoySnapshot()
	}
}

func (c *k8sCluster) updatedIngress(oldObj interface{}, newObj interface{}) {
	oldIngress := oldObj.(*extbeta1.Ingress)
	log.Info("updated k8s ingress  --> " + c.name + ":" + oldIngress.Namespace + ":" + oldIngress.Name)
	createEnvoySnapshot()
}

func (c *k8sCluster) deletedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Info("deleted k8s ingress  --> " + c.name + ":" + ingress.Namespace + ":" + ingress.Name)
	//TODO: delete from the intialIngress cache
	createEnvoySnapshot()
}

func (c *k8sCluster) watchNodes(informerFactory informers.SharedInformerFactory, resyncPeriod time.Duration) {
	c.nodeInformer = informerFactory.Core().V1().Nodes().Informer()
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
		createEnvoySnapshot()
	}
}

func (c *k8sCluster) deletedNode(obj interface{}) {
	node := obj.(*v1.Node)
	log.Info("deleted k8s node  --> " + c.name + " " + node.Name)
	//TODO: delete from the intialNodes cache
	createEnvoySnapshot()
}

func (c *k8sCluster) watchServices(informerFactory informers.SharedInformerFactory, resyncPeriod time.Duration) {
	c.serviceInformer = informerFactory.Core().V1().Services().Informer()
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
		createEnvoySnapshot()
	}
}

func (c *k8sCluster) updatedService(oldObj interface{}, newObj interface{}) {
	service := oldObj.(*v1.Service)
	log.Info("updated k8s services  --> " + c.name + ":" + service.Namespace + ":" + service.Name)
	createEnvoySnapshot()
}

func (c *k8sCluster) deletedService(obj interface{}) {
	service := obj.(*v1.Service)
	log.Info("deleted k8s services  --> " + c.name + ":" + service.Namespace + ":" + service.Name)
	//TODO: delete from the intialServices cache
	createEnvoySnapshot()
}

// NewKubeClient k8s client.
func newKubeClient(kubeconfigPath string, context string) (kubernetes.Interface, error) {

	var config *restclient.Config

	if kubeconfigPath == "" {
		log.Warn("--kube-config is not specified.  Using the inClusterConfig.")
		kubeconfig, err := restclient.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("error creating inClusterConfig, falling back to default config: %s", err)
		}
		config = kubeconfig
	} else {
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
	}

	// Use protobufs for communication with apiserver.
	config.ContentType = "application/vnd.kubernetes.protobuf"

	return kubernetes.NewForConfig(config)
}
