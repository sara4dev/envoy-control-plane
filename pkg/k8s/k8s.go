package k8s

import (
	"fmt"
	"git.target.com/Kubernetes/envoy-control-plane/pkg/data"
	"git.target.com/Kubernetes/envoy-control-plane/pkg/envoy"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"strings"
	"time"
)

var _envoyCluster *envoy.EnvoyCluster

type K8sCluster struct {
	Context          string `yaml:"context"`
	Zone             string `yaml:"zone"`
	priority         uint32
	clientSet        kubernetes.Interface
	ingressInformer  cache.SharedInformer
	initialIngresses []string
	serviceInformer  cache.SharedInformer
	initialServices  []string
	secretInformer   cache.SharedInformer
	initialSecrets   []string
	nodeInformer     cache.SharedInformer
	initialNodes     []string
}

var err error

func RunK8sControllers(envoyCluster *envoy.EnvoyCluster, k8sClusters []*K8sCluster, zone string, kubeConfigPath string, resyncPeriod time.Duration) {
	_envoyCluster = envoyCluster
	envoyCluster.K8sCacheStoreMap = make(map[string]*data.K8sCacheStore)

	for _, k8sCluster := range k8sClusters {
		k8sCluster.setClusterPriority(zone)
		k8sCacheStore := data.K8sCacheStore{
			Name:     k8sCluster.Context,
			Zone:     k8sCluster.Zone,
			Priority: k8sCluster.priority,
		}
		envoyCluster.K8sCacheStoreMap[k8sCluster.Context] = &k8sCacheStore
		err = k8sCluster.startK8sControllers(kubeConfigPath, envoyCluster, resyncPeriod)
		if err != nil {
			log.Fatal("Fatal Error occurred: " + err.Error())
		}
	}
	//Create the first snapshot once the cache store is updated
	envoyCluster.CreateEnvoySnapshot()

	//Add the events to k8s controllers to start watching the events
	for _, k8sCluster := range k8sClusters {
		k8sCluster.addK8sEventHandlers()
	}

}

func (c *K8sCluster) setClusterPriority(envoyZone string) {
	if strings.ToLower(c.Zone) == strings.ToLower(envoyZone) {
		c.priority = 0
	} else {
		c.priority = 1
	}

}

func (c *K8sCluster) startK8sControllers(kubeConfigPath string, envoyCluster *envoy.EnvoyCluster, resyncPeriod time.Duration) error {
	c.clientSet, err = newKubeClient(kubeConfigPath, c.Context)
	if err != nil {
		return err
	}
	watchNamespaces := v1.NamespaceAll
	//watchNamespaces = "kube-system"
	c.watchObjects(resyncPeriod, &v1beta1.Ingress{}, envoyCluster, watchNamespaces)
	c.watchObjects(resyncPeriod, &v1.Service{}, envoyCluster, watchNamespaces)
	c.watchObjects(resyncPeriod, &v1.Secret{}, envoyCluster, watchNamespaces)
	c.watchObjects(resyncPeriod, &v1.Node{}, envoyCluster, watchNamespaces)
	return nil
}

func (c *K8sCluster) addK8sEventHandlers() {
	c.ingressInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedObj,
		UpdateFunc: c.updatedObj,
		DeleteFunc: c.deletedObj,
	})

	c.serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedObj,
		UpdateFunc: c.updatedObj,
		DeleteFunc: c.deletedObj,
	})

	c.secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedObj,
		UpdateFunc: c.updatedObj,
		DeleteFunc: c.deletedObj,
	})

	c.nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addedObj,
		DeleteFunc: c.deletedObj,
	})
}

func (c *K8sCluster) watchObjects(resyncPeriod time.Duration, objType runtime.Object, envoyCluster *envoy.EnvoyCluster, watchNamespaces string) {
	var lw *cache.ListWatch
	var informer *cache.SharedInformer
	var cacheStore *cache.Store
	var initialObjects *[]string
	switch objType.(type) {
	case *v1beta1.Ingress:
		lw = cache.NewListWatchFromClient(c.clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", watchNamespaces, fields.Everything())
		informer = &c.ingressInformer
		cacheStore = &envoyCluster.K8sCacheStoreMap[c.Context].IngressCacheStore
		initialObjects = &c.initialIngresses
	case *v1.Service:
		lw = cache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "services", watchNamespaces, fields.Everything())
		informer = &c.serviceInformer
		cacheStore = &envoyCluster.K8sCacheStoreMap[c.Context].ServiceCacheStore
		initialObjects = &c.initialServices
	case *v1.Secret:
		lw = cache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "secrets", watchNamespaces, fields.Everything())
		informer = &c.secretInformer
		cacheStore = &envoyCluster.K8sCacheStoreMap[c.Context].SecretCacheStore
		initialObjects = &c.initialSecrets
	case *v1.Node:
		lw = cache.NewListWatchFromClient(c.clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
		informer = &c.nodeInformer
		cacheStore = &envoyCluster.K8sCacheStoreMap[c.Context].NodeCacheStore
		initialObjects = &c.initialNodes
	}

	*informer = cache.NewSharedInformer(lw, objType, resyncPeriod)
	*cacheStore = (*informer).GetStore()
	go (*informer).Run(wait.NeverStop)
	log.Infof("waiting to sync ingress for cluster: %v", c.Context)
	for !(*informer).HasSynced() {
	}
	*initialObjects = (*cacheStore).ListKeys()
	log.Infof("%v %T synced for cluster %v", len((*cacheStore).List()), objType, c.Context)
}

func (c *K8sCluster) addedObj(obj interface{}) {
	var objNamespace string
	var objName string
	var initialObjects *[]string

	switch newObj := obj.(type) {
	case *v1beta1.Ingress:
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
			log.Infof("skipping envoy updates, as it's not a TLS secret --> %v:%v:%v", c.Context, objNamespace, objName)
			return
		}
	case *v1.Node:
		objNamespace = newObj.Namespace
		objName = newObj.Name
		initialObjects = &c.initialNodes
	}

	for _, key := range *initialObjects {
		namespace, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			log.Fatalf("Error while splitting the metanamespacekey %v in the cluster %v", key, c.Context)
		}
		if objNamespace == namespace && objName == name {
			log.Infof("skipping envoy updates, as it's an existing %T --> %v:%v:%v", obj, c.Context, objNamespace, objName)
			return
		}
	}

	log.Infof("added k8s %T  --> %v:%v:%v", obj, c.Context, objNamespace, objName)
	_envoyCluster.CreateEnvoySnapshot()
	//Add new keys back to initial list, so that we don't end up creating envoySnapshot during resync
	newkey := objName
	if objNamespace != "" {
		newkey = objNamespace + "/" + objName
	}
	*initialObjects = append(*initialObjects, newkey)
}

func (c *K8sCluster) updatedObj(oldObj interface{}, newObj interface{}) {
	var objNamespace string
	var objName string

	switch updatedObj := oldObj.(type) {
	case *v1beta1.Ingress:
		objNamespace = updatedObj.Namespace
		objName = updatedObj.Name
	case *v1.Service:
		objNamespace = updatedObj.Namespace
		objName = updatedObj.Name
	case *v1.Secret:
		objNamespace = updatedObj.Namespace
		objName = updatedObj.Name
		if updatedObj.Data["tls.crt"] == nil || updatedObj.Data["tls.key"] == nil {
			log.Infof("skipping envoy updates, as it's not a TLS secret --> %v:%v:%v", c.Context, objNamespace, objName)
			return
		}
	case *v1.Node:
		objNamespace = updatedObj.Namespace
		objName = updatedObj.Name
	}

	if cmp.Equal(oldObj, newObj,
		cmpopts.IgnoreFields(v1beta1.Ingress{}, "Status"),
		cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")) {
		log.Infof("skipping envoy updates, as only %T status has changed --> %v:%v:%v", oldObj, c.Context, objNamespace, objName)
		return
	}
	log.Infof("updated k8s %T --> %v:%v:%v", oldObj, c.Context, objNamespace, objName)
	_envoyCluster.CreateEnvoySnapshot()
}

func (c *K8sCluster) deletedObj(obj interface{}) {
	var objNamespace string
	var objName string
	var initialObjects *[]string

	switch delObj := obj.(type) {
	case *v1beta1.Ingress:
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
			log.Infof("skipping envoy updates, as it's not a TLS secret --> %v:%v:%v", c.Context, objNamespace, objName)
			return
		}
	case *v1.Node:
		objNamespace = delObj.Namespace
		objName = delObj.Name
		initialObjects = &c.initialNodes
	}

	log.Infof("deleted k8s %T  --> %v:%v:%v", obj, c.Context, objNamespace, objName)
	var index int
	var key string
	for index, key = range *initialObjects {
		namespace, name, err := cache.SplitMetaNamespaceKey(key)
		if err != nil {
			log.Fatal("Error while splittig the metanamespace key")
		}
		if objNamespace == namespace && objName == name {
			//delete from the intialObjects cache
			*initialObjects = append((*initialObjects)[:index], (*initialObjects)[index+1:]...)
			break
		}
	}
	_envoyCluster.CreateEnvoySnapshot()
}

// NewKubeClient k8s client.
func newKubeClient(kubeconfigPath string, context string) (kubernetes.Interface, error) {
	var config *rest.Config
	apiConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("error reading kube config from file: %s", err)
	}
	if apiConfig.Contexts[context] == nil {
		log.Fatalf("Context %v is not found in the kube config file %v", context, kubeconfigPath)
	}
	authInfo := apiConfig.Contexts[context].AuthInfo
	token := apiConfig.AuthInfos[authInfo].Token
	cluster := apiConfig.Contexts[context].Cluster
	server := apiConfig.Clusters[cluster].Server
	config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{AuthInfo: api.AuthInfo{Token: token}, ClusterInfo: api.Cluster{Server: server}}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating client config: %s", err)
	}

	// Use protobufs for communication with apiserver.
	config.ContentType = "application/vnd.kubernetes.protobuf"

	return kubernetes.NewForConfig(config)
}
