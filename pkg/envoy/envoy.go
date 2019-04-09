package envoy

import (
	"fmt"
	"git.target.com/Kubernetes/envoy-control-plane/pkg/data"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/envoyproxy/go-control-plane/pkg/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"net"
	"sync"
	"sync/atomic"
)

const grpcMaxConcurrentStreams = 2147483647

type EnvoyCluster struct {
	EnvoySnapshotCache cache.SnapshotCache
	Version            int32
	tlsDataCache       sync.Map
	K8sCacheStoreMap   map[string]*data.K8sCacheStore
	DefaultTlsSecret   string
}

type k8sService struct {
	name        string
	ingressName string
	namespace   string
	ingressHost string
	port        int32
}

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

func NewEnvoyCluster() *EnvoyCluster {
	envoyCluster := EnvoyCluster{}
	envoyCluster.EnvoySnapshotCache = cache.NewSnapshotCache(false, Hasher{}, logger{})
	return &envoyCluster
}

func (e *EnvoyCluster) RunManagementServer(ctx context.Context, port uint) {
	// gRPC golang library sets a very small upper bound for the number gRPC/h2
	// streams over a single TCP connection. If a proxy multiplexes requests over
	// a single connection to the management server, then it might lead to
	// availability problems.
	var grpcOptions []grpc.ServerOption
	signal := make(chan struct{})
	cb := &callbacks{signal: signal}
	server := server.NewServer(e.EnvoySnapshotCache, cb)
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

func (e *EnvoyCluster) CreateEnvoySnapshot() {
	atomic.AddInt32(&e.Version, 1)
	//TODO Get the nodeID dynamically
	//nodeId := EnvoySnapshotCache.GetStatusKeys()[0]
	log.Infof(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>> creating snapshot Version " + fmt.Sprint(e.Version))

	envoyListenersChan := make(chan []cache.Resource)
	envoyClustersChan := make(chan []cache.Resource)
	envoyEndpointsChan := make(chan []cache.Resource)

	go e.makeEnvoyListeners(envoyListenersChan)
	go e.makeEnvoyClusters(envoyClustersChan)
	go e.makeEnvoyEndpoints(envoyEndpointsChan)

	envoyListeners := <-envoyListenersChan
	envoyEndpoints := <-envoyEndpointsChan
	envoyClusters := <-envoyClustersChan

	snap := cache.NewSnapshot(fmt.Sprint(e.Version), envoyEndpoints, envoyClusters, nil, envoyListeners)

	//TODO Get the nodeID dynamically
	e.EnvoySnapshotCache.SetSnapshot("k8s_ingress", snap)
	log.Infof(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>> created snapshot Version " + fmt.Sprint(e.Version))
}

func getClusterName(k8sNamespace string, k8singressHost string, k8sServiceName string, k8sServicePort int32) string {
	return k8sNamespace + ":" + k8singressHost + ":" + k8sServiceName + ":" + fmt.Sprint(k8sServicePort)
}
