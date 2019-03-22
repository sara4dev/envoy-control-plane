package main

import (
	"testing"
)

//func init() {
//	k8sClusters = []*k8sCluster{
//		{
//			name: "tgt-ttc-bigoli-test",
//			zone: TTC,
//		},
//		{
//			name: "tgt-tte-bigoli-test",
//			zone: TTE,
//		},
//	}
//}

func TestStartK8sControllers(t *testing.T) {
	for _, k8sCluster := range k8sClusters {
		err = k8sCluster.startK8sControllers("")
		if err == nil {
			t.Error("Expecting Error but got nothing")
		}
	}
}

//func TestWatchIngresses(t *testing.T) {
//	for _, k8sCluster := range k8sClusters {
//		k8sCluster.clientSet = fake.NewSimpleClientset(&extbeta1.Ingress{})
//		//source := fcache.NewFakeControllerSource()
//		//k8sCluster.ingressInformer = k8scache.NewSharedInformer(source, &extbeta1.Ingress{}, 1*time.Second)
//		//source.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}})
//		//source.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod2"}})
//		//
//		//// create the shared informer and resync every 1s
//		//informer := NewSharedInformer(source, &v1.Pod{}, 1*time.Second).(*sharedIndexInformer)
//		k8sCluster.watchIngresses(time.Second * 1)
//	}
//}
//
//func TestAddK8sEventHandlers(t *testing.T) {
//	for _, k8sCluster := range k8sClusters {
//		k8sCluster.addK8sEventHandlers()
//	}
//}
