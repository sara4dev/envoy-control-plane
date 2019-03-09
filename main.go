package main

import (
	"os"

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"

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
	runtime.GOMAXPROCS(2)
	//TODO how to update the cache if the TLS changes?
	tlsDataCache = make(map[string]auth.TlsCertificate)
	clientSet, err = newKubeClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		log.Fatalf("error newKubeClient: %s", err.Error())
	}
	ingressWatchlist := k8scache.NewListWatchFromClient(clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", v1.NamespaceAll, fields.Everything())
	watchIngresses(ingressWatchlist, resyncPeriod)

	nodeWatchlist := k8scache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
	watchNodes(nodeWatchlist, resyncPeriod)

	serviceWatchlist := k8scache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "services", v1.NamespaceAll, fields.Everything())
	watchServices(serviceWatchlist, resyncPeriod)

	signal = make(chan struct{})
	cb := &callbacks{signal: signal}
	envoySnapshotCache = envoycache.NewSnapshotCache(false, Hasher{}, logger{})
	srv := server.NewServer(envoySnapshotCache, cb)

	RunManagementServer(context.Background(), srv, 8080)
}
