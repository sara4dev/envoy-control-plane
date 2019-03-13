package main

import (
	"k8s.io/api/core/v1"
	"os"
	"strconv"
	"strings"

	"github.com/urfave/cli"

	"k8s.io/client-go/kubernetes"

	"runtime"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	extbeta1 "k8s.io/api/extensions/v1beta1"
)

type k8sController struct {
	clusterName string
}

func main() {
	app := cli.NewApp()
	app.Name = "envoy-control-plane"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "kube-config, k",
			Usage: "Path to kube config to use",
			Value: "",
		},
		cli.StringFlag{
			Name:  "zone, z",
			Usage: "zone where envoy is deployed TTC/TTE",
			Value: "ttc",
		},
	}
	app.Action = run
	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("Failed to run control plane: %s", err)
		os.Exit(1)
	}
}

func setClusterPriority(envoyZone string) {
	for _, k8sCluster := range k8sClusters {
		if strings.ToLower(strconv.Itoa(int(k8sCluster.zone))) == strings.ToLower(envoyZone) {
			k8sCluster.priority = 0
		} else {
			k8sCluster.priority = 1
		}
	}
}

func run(ctx *cli.Context) error {
	runtime.GOMAXPROCS(4)
	//TODO how to update the cache if the TLS changes?
	ingressLists = make(map[*k8sCluster]*extbeta1.IngressList)
	nodeLists = make(map[*k8sCluster]*v1.NodeList)
	serviceLists = make(map[*k8sCluster]*v1.ServiceList)
	k8sClusters = []k8sCluster{
		{
			name: "tgt-ttc-bigoli-test",
			zone: TTC,
		},
		{
			name: "tgt-tte-bigoli-test",
			zone: TTE,
		},
	}
	setClusterPriority(ctx.String("zone"))
	clientSets = make(map[string]kubernetes.Interface)
	for _, k8sCluster := range k8sClusters {
		//k8sSyncController := k8sController{clusterName:k8sCluster}
		clientSet, err := newKubeClient(ctx.String("kube-config"), k8sCluster.name)
		clientSets[k8sCluster.name] = clientSet
		if err != nil {
			log.Fatalf("error newKubeClient: %s", err.Error())
		}

		//ingressWatchlist := k8scache.NewListWatchFromClient(clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", v1.NamespaceAll, fields.Everything())
		k8sCluster.watchIngresses(clientSet, resyncPeriod)

		//nodeWatchlist := k8scache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
		k8sCluster.watchNodes(clientSet, resyncPeriod)

		//serviceWatchlist := k8scache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "services", v1.NamespaceAll, fields.Everything())
		k8sCluster.watchServices(clientSet, resyncPeriod)
	}

	signal = make(chan struct{})
	cb := &callbacks{signal: signal}
	envoySnapshotCache = envoycache.NewSnapshotCache(false, Hasher{}, logger{})
	srv := server.NewServer(envoySnapshotCache, cb)
	createEnvoySnapshot()
	RunManagementServer(context.Background(), srv, 8080)
	return nil
}
