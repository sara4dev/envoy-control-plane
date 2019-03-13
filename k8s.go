package main

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	name     string
	zone     zone
	priority uint32
}

var (
	k8sClusters  []k8sCluster
	clientSets   map[string]kubernetes.Interface
	resyncPeriod time.Duration
	signal       chan struct{}
	err          error
	ingressLists map[*k8sCluster]*extbeta1.IngressList
	nodeLists    map[*k8sCluster]*v1.NodeList
	serviceLists map[*k8sCluster]*v1.ServiceList
)

func (c *k8sCluster) watchIngresses(clientSet kubernetes.Interface, resyncPeriod time.Duration) {
	ingressLists[c], err = clientSet.ExtensionsV1beta1().Ingresses(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		log.Fatal("Error to list ingresses")
	}

	factory := informers.NewSharedInformerFactory(clientSet, resyncPeriod)
	ingressSharedInformer := factory.Extensions().V1beta1().Ingresses().Informer()
	ingressSharedInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedIngress,
		UpdateFunc: c.updatedIngress,
		DeleteFunc: c.deletedIngress,
	})
	go ingressSharedInformer.Run(wait.NeverStop)
}

func (c *k8sCluster) addedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	var isExistingIngress bool

	for _, initialIngress := range ingressLists[c].Items {
		if initialIngress.Namespace == ingress.Namespace && initialIngress.Name == ingress.Name {
			isExistingIngress = true
			break
		}
	}

	if !isExistingIngress {
		log.Info("added k8s ingress :" + ingress.Name)
		ingressLists[c].Items = append(ingressLists[c].Items, *ingress)
		createEnvoySnapshot()
	}
}

func (c *k8sCluster) updatedIngress(oldObj interface{}, newObj interface{}) {
	ingress := oldObj.(*extbeta1.Ingress)
	log.Info("updated k8s ingress :" + ingress.Name)
	createEnvoySnapshot()
}

func (c *k8sCluster) deletedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Info("deleted k8s ingress :" + ingress.Name)
	createEnvoySnapshot()
}

func (c *k8sCluster) watchNodes(clientSet kubernetes.Interface, resyncPeriod time.Duration) {
	nodeLists[c], err = clientSet.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		log.Fatal("Error to list nodes")
	}

	factory := informers.NewSharedInformerFactory(clientSet, resyncPeriod)
	nodeSharedInformer := factory.Core().V1().Nodes().Informer()
	nodeSharedInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedNode,
		DeleteFunc: c.deletedNode,
	})

	//Run the controller as a goroutine
	go nodeSharedInformer.Run(wait.NeverStop)
}

func (c *k8sCluster) addedNode(obj interface{}) {
	//err := ingressK8sCacheStores.Add(obj)
	node := obj.(*v1.Node)

	var isExistingNode bool

	for _, initialNode := range nodeLists[c].Items {
		if initialNode.Name == node.Name {
			isExistingNode = true
			break
		}
	}

	if !isExistingNode {
		log.Info("added k8s node --> " + c.name + "" + node.Name)
		nodeLists[c].Items = append(nodeLists[c].Items, *node)
		createEnvoySnapshot()
	}
}

func (c *k8sCluster) deletedNode(obj interface{}) {
	node := obj.(*v1.Node)
	log.Info("deleted k8s node :" + node.Name)
	createEnvoySnapshot()
}

func (c *k8sCluster) watchServices(clientSet kubernetes.Interface, resyncPeriod time.Duration) {
	serviceLists[c], err = clientSet.CoreV1().Services(v1.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		log.Fatal("Error to list services")
	}

	factory := informers.NewSharedInformerFactory(clientSet, resyncPeriod)
	serviceSharedInformer := factory.Core().V1().Services().Informer()
	serviceSharedInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedService,
		UpdateFunc: c.updatedService,
		DeleteFunc: c.deletedService,
	})

	//Run the controller as a goroutine
	go serviceSharedInformer.Run(wait.NeverStop)
}

func (c *k8sCluster) addedService(obj interface{}) {
	service := obj.(*v1.Service)

	var isExistingService bool

	for _, initialNode := range serviceLists[c].Items {
		if initialNode.Name == service.Name {
			isExistingService = true
			break
		}
	}

	if !isExistingService {
		log.Info("added k8s service --> " + c.name + "" + service.Name)
		serviceLists[c].Items = append(serviceLists[c].Items, *service)
		createEnvoySnapshot()
	}
}

func (c *k8sCluster) updatedService(oldObj interface{}, newObj interface{}) {
	service := oldObj.(*v1.Service)
	log.Info("updated service node :" + service.Name)
	createEnvoySnapshot()
}

func (c *k8sCluster) deletedService(obj interface{}) {
	service := obj.(*v1.Service)
	log.Info("deleted service node :" + service.Name)
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
