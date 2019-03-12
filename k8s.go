package main

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	k8sClusters           []string
	clientSets            []kubernetes.Interface
	resyncPeriod          time.Duration
	signal                chan struct{}
	ingressK8sCacheStores []k8scache.Store
	nodeK8sCacheStores    []k8scache.Store
	serviceK8sCacheStores []k8scache.Store
	err                   error
)

func watchIngresses(watchlist *k8scache.ListWatch, resyncPeriod time.Duration) {
	//Setup an informer to call functions when the watchlist changes
	//var ingressK8sCacheStore k8scache.Store
	ingressK8sCacheStore, ingressK8sController := k8scache.NewInformer(
		watchlist,
		&extbeta1.Ingress{},
		resyncPeriod,
		k8scache.ResourceEventHandlerFuncs{
			AddFunc:    addedIngress,
			UpdateFunc: updatedIngress,
			DeleteFunc: deletedIngress,
		},
	)
	ingressK8sCacheStores = append(ingressK8sCacheStores, ingressK8sCacheStore)
	//Run the controller as a goroutine
	go ingressK8sController.Run(wait.NeverStop)
}

func addedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Info("added k8s ingress :" + ingress.Name)
	createEnvoySnapshot()
}

func updatedIngress(oldObj interface{}, newObj interface{}) {
	ingress := oldObj.(*extbeta1.Ingress)
	log.Info("updated k8s ingress :" + ingress.Name)
	createEnvoySnapshot()
}

func deletedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Info("deleted k8s ingress :" + ingress.Name)
	createEnvoySnapshot()
}

func watchNodes(watchlist *k8scache.ListWatch, resyncPeriod time.Duration) {
	//Setup an informer to call functions when the watchlist changes
	nodeK8sCacheStore, nodeK8sController := k8scache.NewInformer(
		watchlist,
		&v1.Node{},
		resyncPeriod,
		k8scache.ResourceEventHandlerFuncs{
			AddFunc:    addedNode,
			DeleteFunc: deletedNode,
		},
	)
	nodeK8sCacheStores = append(nodeK8sCacheStores, nodeK8sCacheStore)
	//Run the controller as a goroutine
	go nodeK8sController.Run(wait.NeverStop)
}

func addedNode(obj interface{}) {
	//err := ingressK8sCacheStores.Add(obj)
	node := obj.(*v1.Node)
	log.Info("added k8s node :" + node.Name)
	//createEnvoySnapshot()
}

func deletedNode(obj interface{}) {
	node := obj.(*v1.Node)
	log.Info("deleted k8s node :" + node.Name)
	//createEnvoySnapshot()
}

func watchServices(k8sCluster string, watchlist *k8scache.ListWatch, resyncPeriod time.Duration) {
	//Setup an informer to call functions when the watchlist changes
	serviceK8sCacheStore, serviceK8sController := k8scache.NewInformer(
		watchlist,
		&v1.Service{},
		resyncPeriod,
		k8scache.ResourceEventHandlerFuncs{
			AddFunc:    addedService,
			UpdateFunc: updatedService,
			DeleteFunc: deletedService,
		},
	)
	serviceK8sCacheStores = append(serviceK8sCacheStores, serviceK8sCacheStore)
	//Run the controller as a goroutine
	//Run the controller as a goroutine
	go serviceK8sController.Run(wait.NeverStop)
}

func addedService(obj interface{}) {
	//err := ingressK8sCacheStores.Add(obj)
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
func newKubeClient(kubeconfigPath string, context string) (kubernetes.Interface, error) {

	var config *restclient.Config

	if kubeconfigPath == "" {
		log.Warn("--kubeconfig is not specified.  Using the inClusterConfig.")
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
