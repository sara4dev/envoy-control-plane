package main

import (
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"strconv"
	"time"
)

var (
	clientSet            kubernetes.Interface
	resyncPeriod         time.Duration
	signal               chan struct{}
	ingressK8sCacheStore k8scache.Store
	ingressK8sController k8scache.Controller
	nodeK8sCacheStore    k8scache.Store
	nodeK8sController    k8scache.Controller
	serviceK8sCacheStore k8scache.Store
	serviceK8sController k8scache.Controller
	err                  error
)

func watchIngresses(watchlist *k8scache.ListWatch, resyncPeriod time.Duration) k8scache.Store {
	//Setup an informer to call functions when the watchlist changes
	ingressK8sCacheStore, ingressK8sController = k8scache.NewInformer(
		watchlist,
		&extbeta1.Ingress{},
		resyncPeriod,
		k8scache.ResourceEventHandlerFuncs{
			AddFunc:    addedIngress,
			UpdateFunc: updatedIngress,
			DeleteFunc: deletedIngress,
		},
	)
	//Run the controller as a goroutine
	go ingressK8sController.Run(wait.NeverStop)
	return ingressK8sCacheStore
}

func addedIngress(obj interface{}) {
	//err := ingressK8sCacheStore.Add(obj)
	ingress := obj.(*extbeta1.Ingress)
	log.Info("added k8s ingress :" + ingress.Name)
	log.Info("Cache count :" + strconv.Itoa(len(ingressK8sCacheStore.List())))
	createEnvoySnapshot()
}

func updatedIngress(oldObj interface{}, newObj interface{}) {
	ingress := oldObj.(*extbeta1.Ingress)
	log.Info("updated k8s ingress :" + ingress.Name)
	//err := ingressK8sCacheStore.Delete(oldObj)
	//log.Error(err)
	//err = ingressK8sCacheStore.Add(newObj)
	//log.Error(err)
	createEnvoySnapshot()
}

func deletedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Info("deleted k8s ingress :" + ingress.Name)
	//err := ingressK8sCacheStore.Delete(obj)
	//log.Error(err)
	createEnvoySnapshot()
}

func watchNodes(watchlist *k8scache.ListWatch, resyncPeriod time.Duration) k8scache.Store {
	//Setup an informer to call functions when the watchlist changes
	nodeK8sCacheStore, nodeK8sController = k8scache.NewInformer(
		watchlist,
		&v1.Node{},
		resyncPeriod,
		k8scache.ResourceEventHandlerFuncs{
			AddFunc:    addedNode,
			DeleteFunc: deletedNode,
		},
	)
	//Run the controller as a goroutine
	go nodeK8sController.Run(wait.NeverStop)
	return nodeK8sCacheStore
}

func addedNode(obj interface{}) {
	//err := ingressK8sCacheStore.Add(obj)
	node := obj.(*v1.Node)
	log.Info("added k8s node :" + node.Name)
	//createEnvoySnapshot()
}

func deletedNode(obj interface{}) {
	node := obj.(*v1.Node)
	log.Info("deleted k8s node :" + node.Name)
	//createEnvoySnapshot()
}

func watchServices(watchlist *k8scache.ListWatch, resyncPeriod time.Duration) k8scache.Store {
	//Setup an informer to call functions when the watchlist changes
	serviceK8sCacheStore, serviceK8sController = k8scache.NewInformer(
		watchlist,
		&v1.Service{},
		resyncPeriod,
		k8scache.ResourceEventHandlerFuncs{
			AddFunc:    addedService,
			UpdateFunc: updatedService,
			DeleteFunc: deletedService,
		},
	)
	//Run the controller as a goroutine
	go serviceK8sController.Run(wait.NeverStop)
	return serviceK8sCacheStore
}

func addedService(obj interface{}) {
	//err := ingressK8sCacheStore.Add(obj)
	service := obj.(*v1.Service)
	log.Info("added service node :" + service.Name)
	//createEnvoySnapshot()
}

func updatedService(oldObj interface{}, newObj interface{}) {
	service := oldObj.(*v1.Service)
	log.Info("updated service node :" + service.Name)
	//createEnvoySnapshot()
}

func deletedService(obj interface{}) {
	service := obj.(*v1.Service)
	log.Info("deleted service node :" + service.Name)
	//createEnvoySnapshot()
}

// NewKubeClient k8s client.
func newKubeClient(kubeconfigPath string) (kubernetes.Interface, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath == "" {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}
	// Use protobufs for communication with apiserver.
	config.ContentType = "application/vnd.kubernetes.protobuf"

	return kubernetes.NewForConfig(config)
}
