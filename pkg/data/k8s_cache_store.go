package data

import k8scache "k8s.io/client-go/tools/cache"

type K8sCacheStore struct {
	Name              string
	Zone              Zone
	Priority          uint32
	IngressCacheStore k8scache.Store
	ServiceCacheStore k8scache.Store
	SecretCacheStore  k8scache.Store
	NodeCacheStore    k8scache.Store
}

type Zone int

//TODO remove hardcoded zone
const (
	TTC Zone = 0
	TTE Zone = 1
)
