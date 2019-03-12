package main

import (
	"k8s.io/client-go/kubernetes"
	"os"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	k8scache "k8s.io/client-go/tools/cache"
	"runtime"
)

func main() {
	log.Info("started main")
	runtime.GOMAXPROCS(4)
	//TODO how to update the cache if the TLS changes?
	//tlsDataCache = make(map[string]auth.TlsCertificate)
	ingressK8sCacheStores = []k8scache.Store{}
	nodeK8sCacheStores = []k8scache.Store{}
	serviceK8sCacheStores = []k8scache.Store{}
	k8sClusters = []string{"tgt-ttc-bigoli-test", "tgt-tte-bigoli-test"}
	clientSets = []kubernetes.Interface{}
	for _, k8sCluster := range k8sClusters {
		clientSet, err := newKubeClient(os.Getenv("KUBECONFIG"), k8sCluster)
		clientSets = append(clientSets, clientSet)
		if err != nil {
			log.Fatalf("error newKubeClient: %s", err.Error())
		}

		ingressWatchlist := k8scache.NewListWatchFromClient(clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", v1.NamespaceAll, fields.Everything())
		watchIngresses(ingressWatchlist, resyncPeriod)

		nodeWatchlist := k8scache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
		watchNodes(nodeWatchlist, resyncPeriod)

		serviceWatchlist := k8scache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "services", v1.NamespaceAll, fields.Everything())
		watchServices(k8sCluster, serviceWatchlist, resyncPeriod)
	}

	signal = make(chan struct{})
	cb := &callbacks{signal: signal}
	envoySnapshotCache = envoycache.NewSnapshotCache(false, Hasher{}, logger{})
	srv := server.NewServer(envoySnapshotCache, cb)

	RunManagementServer(context.Background(), srv, 8080)
}
