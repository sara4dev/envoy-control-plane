package main

import (
	"k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"net/http"
	"time"
)

var (
	resyncPeriod time.Duration
)

func main() {
	clientset, err := newKubeClient("/Users/z0027kp/.kube/config")
	if err != nil {
		log.Fatal("error")
	}

	watchlist := cache.NewListWatchFromClient(clientset.ExtensionsV1beta1().RESTClient(), "ingresses", v1.NamespaceAll, fields.Everything())

	watchIngresses(watchlist, resyncPeriod)
	log.Fatal(http.ListenAndServe(":8000", nil))
}

func watchIngresses(watchlist *cache.ListWatch, resyncPeriod time.Duration) cache.Store {
	//Setup an informer to call functions when the watchlist changes
	eStore, eController := cache.NewInformer(
		watchlist,
		&extbeta1.Ingress{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    addedIngress,
			DeleteFunc: deletedIngress,
		},
	)
	//Run the controller as a goroutine
	go eController.Run(wait.NeverStop)
	return eStore
}

func addedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Println("created ingress:" + ingress.Name)
}

func deletedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Println("deleted ingress:" + ingress.Name)
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
