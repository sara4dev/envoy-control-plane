package main

import (
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
	runtime.GOMAXPROCS(4)
	tlsDataCache = make(map[string]auth.TlsCertificate)
	clientSet, err = newKubeClient("/Users/z0027kp/.kube/config")
	if err != nil {
		log.Fatal("error")
	}
	ingressWatchlist := k8scache.NewListWatchFromClient(clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", "kube-system", fields.Everything())
	watchIngresses(ingressWatchlist, resyncPeriod)

	nodeWatchlist := k8scache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
	watchNodes(nodeWatchlist, resyncPeriod)

	serviceWatchlist := k8scache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "services", "kube-system", fields.Everything())
	watchServices(serviceWatchlist, resyncPeriod)

	signal = make(chan struct{})
	cb := &callbacks{signal: signal}
	envoySnapshotCache = envoycache.NewSnapshotCache(false, Hasher{}, logger{})
	srv := server.NewServer(envoySnapshotCache, cb)

	RunManagementServer(context.Background(), srv, 8080)
}
