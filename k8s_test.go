package main

import "testing"

func init() {
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
}

func TestStartK8sControllers(t *testing.T) {
	for _, k8sCluster := range k8sClusters {
		err = k8sCluster.startK8sControllers("")
		if err == nil {
			t.Error("Expecting Error but got nothing")
		}
	}
}
