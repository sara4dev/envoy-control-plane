package cmd

import (
	"fmt"
	"git.target.com/Kubernetes/envoy-control-plane/pkg/envoy"
	"git.target.com/Kubernetes/envoy-control-plane/pkg/k8s"
	"git.target.com/Kubernetes/envoy-control-plane/pkg/logger"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
	"os"
	"runtime"
	"time"
)

var cfgFile string

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cobra.yaml)")
	rootCmd.PersistentFlags().Bool("viper", true, "Use Viper for configuration")
}

func initConfig() {
	// Don't forget to read config either from cfgFile or from home directory!
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".cobra" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".cobra")
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Can't read config:", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "envoy-control-plane",
	Short: "Envoyproxy control plane for on-premise multi cluster ingress",
	Long:  `Envoyproxy control plane for on-premise multi cluster ingress`,
	Run:   run,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	logger.LogSetup(viper.GetString("logLevel"))
	runtime.GOMAXPROCS(2)
	log.Info("started...")
	var k8sClusters []*k8s.K8sCluster
	err := viper.UnmarshalKey("clusters", &k8sClusters)
	if err != nil {
		log.Fatal("Error in parsing the config file.")
	}

	envoyCluster := envoy.NewEnvoyCluster()
	//resync with kubernetes every 60 minutes in case if cache went out of sync
	resyncPeriod := time.Hour * 8
	//Start watching for K8s cluster objects
	k8s.RunK8sControllers(envoyCluster, viper.GetString("kubeConfigPath"), viper.GetString("defaultTlsSecret"), viper.GetString("zone"), k8sClusters, resyncPeriod)
	//Start the gRPC api server for envoy control plane
	envoyCluster.RunManagementServer(context.Background(), 8080)
}
