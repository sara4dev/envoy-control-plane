package main

import (
	"git.target.com/Kubernetes/envoy-control-plane/pkg/envoy"
	"github.com/urfave/cli"
	"os"
	"runtime"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

//type k8sController struct {
//	clusterName string
//}

var envoyCluster envoy.EnvoyCluster

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

func run(ctx *cli.Context) error {
	runtime.GOMAXPROCS(2)
	//resyncPeriod = time.Minute * 1
	envoyCluster := envoy.NewEnvoyCluster()

	RunK8sControllers(ctx, envoyCluster)

	envoyCluster.RunManagementServer(context.Background(), 8080)
	return nil
}
