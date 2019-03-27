package main

import k8scache "k8s.io/client-go/tools/cache"

type K8sCacheStore struct {
	Name              string
	Zone              zone
	Priority          uint32
	IngressCacheStore k8scache.Store
	ServiceCacheStore k8scache.Store
	SecretCacheStore  k8scache.Store
	NodeCacheStore    k8scache.Store
}
