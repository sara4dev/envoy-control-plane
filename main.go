package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli"

	"runtime"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
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
	resyncPeriod = time.Minute * 2
	//ingressLists = make(map[*k8sCluster]*extbeta1.IngressList)
	//nodeLists = make(map[*k8sCluster]*v1.NodeList)
	//serviceLists = make(map[*k8sCluster]*v1.ServiceList)
	k8sClusters = []*k8sCluster{
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
	for _, k8sCluster := range k8sClusters {
		k8sCluster.startK8sControllers(ctx)
	}
	signal = make(chan struct{})
	cb := &callbacks{signal: signal}
	envoySnapshotCache = envoycache.NewSnapshotCache(false, Hasher{}, logger{})
	srv := server.NewServer(envoySnapshotCache, cb)
	createEnvoySnapshot()
	for _, k8sCluster := range k8sClusters {
		k8sCluster.addK8sEventHandlers()
	}
	RunManagementServer(context.Background(), srv, 8080)
	return nil
}
