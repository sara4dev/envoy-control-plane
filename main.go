package main

import (
	"fmt"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"strconv"
	"sync/atomic"

	"k8s.io/api/core/v1"
	extbeta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"net"
	"sync"
	"time"
)

const grpcMaxConcurrentStreams = 1000000

var (
	resyncPeriod         time.Duration
	signal               chan struct{}
	ingressK8sCacheStore k8scache.Store
	eController          k8scache.Controller
)

// Hasher returns node ID as an ID
type Hasher struct {
}

// ID function
func (h Hasher) ID(node *core.Node) string {
	if node == nil {
		return "unknown"
	}
	return node.Id
}

type logger struct{}

func (logger logger) Infof(format string, args ...interface{}) {
	log.Debugf(format, args...)
}
func (logger logger) Errorf(format string, args ...interface{}) {
	log.Errorf(format, args...)
}

type callbacks struct {
	signal   chan struct{}
	fetches  int
	requests int
	mu       sync.Mutex
}

func (cb *callbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	log.WithFields(log.Fields{"fetches": cb.fetches, "requests": cb.requests}).Info("server callbacks")
}
func (cb *callbacks) OnStreamOpen(_ context.Context, id int64, typ string) error {
	log.Debugf("stream %d open for %s", id, typ)
	return nil
}
func (cb *callbacks) OnStreamClosed(id int64) {
	log.Debugf("stream %d closed", id)
}
func (cb *callbacks) OnStreamRequest(int64, *v2.DiscoveryRequest) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.requests++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}
	return nil
}
func (cb *callbacks) OnStreamResponse(int64, *v2.DiscoveryRequest, *v2.DiscoveryResponse) {}
func (cb *callbacks) OnFetchRequest(_ context.Context, req *v2.DiscoveryRequest) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.fetches++
	if cb.signal != nil {
		close(cb.signal)
		cb.signal = nil
	}
	return nil
}
func (cb *callbacks) OnFetchResponse(*v2.DiscoveryRequest, *v2.DiscoveryResponse) {}

var (
	version            int32
	envoySnapshotCache envoycache.SnapshotCache
)

func main() {
	clientSet, err := newKubeClient("/Users/z0027kp/.kube/config")
	if err != nil {
		log.Fatal("error")
	}

	watchlist := k8scache.NewListWatchFromClient(clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", v1.NamespaceAll, fields.Everything())
	watchIngresses(watchlist, resyncPeriod)

	signal = make(chan struct{})
	cb := &callbacks{signal: signal}
	envoySnapshotCache = envoycache.NewSnapshotCache(false, Hasher{}, logger{})
	srv := server.NewServer(envoySnapshotCache, cb)

	RunManagementServer(context.Background(), srv, 8080)
	//log.Info("waiting for events")
	//<-signal
	//createEnvoySnapshot()
}

func watchIngresses(watchlist *k8scache.ListWatch, resyncPeriod time.Duration) k8scache.Store {
	//Setup an informer to call functions when the watchlist changes
	ingressK8sCacheStore, eController = k8scache.NewInformer(
		watchlist,
		&extbeta1.Ingress{},
		resyncPeriod,
		k8scache.ResourceEventHandlerFuncs{
			AddFunc:    addedIngress,
			UpdateFunc: updatedIngress,
			DeleteFunc: deletedIngress,
		},
	)
	//Run the controller as a goroutine
	go eController.Run(wait.NeverStop)
	return ingressK8sCacheStore
}

func addedIngress(obj interface{}) {
	//err := ingressK8sCacheStore.Add(obj)
	ingress := obj.(*extbeta1.Ingress)
	log.Info("added k8s ingress :" + ingress.Name)
	log.Info("Cache count :" + strconv.Itoa(len(ingressK8sCacheStore.List())))
	createEnvoySnapshot()
}

func updatedIngress(oldObj interface{}, newObj interface{}) {
	ingress := oldObj.(*extbeta1.Ingress)
	log.Info("updated k8s ingress :" + ingress.Name)
	//err := ingressK8sCacheStore.Delete(oldObj)
	//log.Error(err)
	//err = ingressK8sCacheStore.Add(newObj)
	//log.Error(err)
	createEnvoySnapshot()
}

func deletedIngress(obj interface{}) {
	ingress := obj.(*extbeta1.Ingress)
	log.Info("deleted k8s ingress :" + ingress.Name)
	//err := ingressK8sCacheStore.Delete(obj)
	//log.Error(err)
	createEnvoySnapshot()
}

// NewKubeClient k8s client.
func newKubeClient(kubeconfigPath string) (kubernetes.Interface, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath == "" {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}
	// Use protobufs for communication with apiserver.
	config.ContentType = "application/vnd.kubernetes.protobuf"

	return kubernetes.NewForConfig(config)
}

func RunManagementServer(ctx context.Context, server server.Server, port uint) {
	// gRPC golang library sets a very small upper bound for the number gRPC/h2
	// streams over a single TCP connection. If a proxy multiplexes requests over
	// a single connection to the management server, then it might lead to
	// availability problems.
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.WithError(err).Fatal("failed to listen")
	}

	// register services
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
	v2.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	v2.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	v2.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	v2.RegisterListenerDiscoveryServiceServer(grpcServer, server)
	discovery.RegisterSecretDiscoveryServiceServer(grpcServer, server)

	log.WithFields(log.Fields{"port": port}).Info("management server listening")
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Error(err)
		}
	}()
	<-ctx.Done()

	grpcServer.GracefulStop()
}

func createEnvoySnapshot() {
	atomic.AddInt32(&version, 1)
	//nodeId := envoySnapshotCache.GetStatusKeys()[0]

	log.Infof(">>>>>>>>>>>>>>>>>>> creating endpoints ")

	//envoyEndpoints := []envoycache.Resource{}
	envoyClusters := []envoycache.Resource{}
	for _, obj := range ingressK8sCacheStore.List() {
		ingress := obj.(*extbeta1.Ingress)
		envoyCluster := v2.Cluster{
			Name:           ingress.Name,
			ConnectTimeout: time.Second * 1,
		}

		envoyClusters = append(envoyClusters, &envoyCluster)

	}

	log.Infof(">>>>>>>>>>>>>>>>>>> creating snapshot Version " + fmt.Sprint(version))

	snap := envoycache.NewSnapshot(fmt.Sprint(version), nil, envoyClusters, nil, nil)

	envoySnapshotCache.SetSnapshot("test-id", snap)
}
