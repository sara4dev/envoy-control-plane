package data

import "k8s.io/client-go/tools/cache"

type K8sCacheStore struct {
	Name              string
	Zone              string
	Priority          uint32
	IngressCacheStore cache.Store
	ServiceCacheStore cache.Store
	SecretCacheStore  cache.Store
	NodeCacheStore    cache.Store
}
