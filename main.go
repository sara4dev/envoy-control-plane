package main

import (
	"os"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	k8scache "k8s.io/client-go/tools/cache"
)

func main() {
	clientSet, err = newKubeClient(os.Getenv("KUBECONFIG"))
	if err != nil {
		log.Fatalf("error newKubeClient: %s", err.Error())
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
