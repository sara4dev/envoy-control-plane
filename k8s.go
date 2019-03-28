package main

import (
	"fmt"
	"git.target.com/Kubernetes/envoy-control-plane/pkg/data"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
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
	"strconv"
	"strings"
	"time"
)

type zone int

//TODO remove hardcoded zone
const (
	TTC zone = 0
	TTE zone = 1
)

type k8sCluster struct {
	name            string
	zone            zone
	priority        uint32
	clientSet       kubernetes.Interface
	ingressInformer k8scache.SharedInformer
	//ingressCacheStore k8scache.Store
	initialIngresses []string
	serviceInformer  k8scache.SharedInformer
	//serviceCacheStore k8scache.Store
	initialServices []string
	secretInformer  k8scache.SharedInformer
	//secretCacheStore  k8scache.Store
	initialSecrets []string
	nodeInformer   k8scache.SharedInformer
	//nodeCacheStore    k8scache.Store
	initialNodes []string
}

var (
	//k8sClusters     	[]*k8sCluster
	resyncPeriod    time.Duration
	err             error
	watchNamespaces string
)

func RunK8sControllers(ctx *cli.Context, envoyCluster EnvoyCluster) {
	envoyCluster.k8sCacheStoreMap = make(map[string]*data.K8sCacheStore)
	k8sClusters := []*k8sCluster{
		{
			name: "tgt-ttc-bigoli-test",
			zone: TTC,
		},
		{
			name: "tgt-tte-bigoli-test",
			zone: TTE,
		},
	}

	for _, k8sCluster := range k8sClusters {
		k8sCluster.setClusterPriority(ctx.String("zone"))
		k8sCacheStore := data.K8sCacheStore{
			Name:     k8sCluster.name,
			Zone:     data.Zone(k8sCluster.zone),
			Priority: k8sCluster.priority,
		}
		envoyCluster.k8sCacheStoreMap[k8sCluster.name] = &k8sCacheStore
		err = k8sCluster.startK8sControllers(ctx.String("kube-config"), envoyCluster)
		if err != nil {
			log.Fatal("Fatal Error occurred: " + err.Error())
		}
	}
	//Create the first snapshot once the cache store is updated
	envoyCluster.createEnvoySnapshot()

	//Add the events to k8s controllers to start watching the events
	for _, k8sCluster := range k8sClusters {
		k8sCluster.addK8sEventHandlers()
	}

}

func (c *k8sCluster) setClusterPriority(envoyZone string) {
	if strings.ToLower(strconv.Itoa(int(c.zone))) == strings.ToLower(envoyZone) {
		c.priority = 0
	} else {
		c.priority = 1
	}

}

func (c *k8sCluster) startK8sControllers(kubeConfigPath string, envoyCluster EnvoyCluster) error {
	c.clientSet, err = newKubeClient(kubeConfigPath, c.name)
	if err != nil {
		return err
	}
	watchNamespaces = v1.NamespaceAll
	//watchNamespaces = "kube-system"
	c.watchObjects(resyncPeriod, &extbeta1.Ingress{}, envoyCluster)
	c.watchObjects(resyncPeriod, &v1.Service{}, envoyCluster)
	c.watchObjects(resyncPeriod, &v1.Secret{}, envoyCluster)
	c.watchObjects(resyncPeriod, &v1.Node{}, envoyCluster)
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

func (c *k8sCluster) watchObjects(resyncPeriod time.Duration, objType runtime.Object, envoyCluster EnvoyCluster) {
	var lw *k8scache.ListWatch
	var informer *k8scache.SharedInformer
	var cacheStore *k8scache.Store
	var initialObjects *[]string
	switch objType.(type) {
	case *extbeta1.Ingress:
		lw = k8scache.NewListWatchFromClient(c.clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", watchNamespaces, fields.Everything())
		informer = &c.ingressInformer
		cacheStore = &envoyCluster.k8sCacheStoreMap[c.name].IngressCacheStore
		initialObjects = &c.initialIngresses
	case *v1.Service:
		lw = k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "services", watchNamespaces, fields.Everything())
		informer = &c.serviceInformer
		cacheStore = &envoyCluster.k8sCacheStoreMap[c.name].ServiceCacheStore
		initialObjects = &c.initialServices
	case *v1.Secret:
		lw = k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "secrets", watchNamespaces, fields.Everything())
		informer = &c.secretInformer
		cacheStore = &envoyCluster.k8sCacheStoreMap[c.name].SecretCacheStore
		initialObjects = &c.initialSecrets
	case *v1.Node:
		lw = k8scache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
		informer = &c.nodeInformer
		cacheStore = &envoyCluster.k8sCacheStoreMap[c.name].NodeCacheStore
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
