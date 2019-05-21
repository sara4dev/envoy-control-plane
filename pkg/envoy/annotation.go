package envoy

import "k8s.io/api/extensions/v1beta1"

const ANNOTATIONS_ROUTEACTION_ROUTE_ENVOYPROXY_HASHPOLICY = "routeaction.route.envoyproxy.io/hashpolicy"

const ANNOTATIONS_THRESHOLDS_CIRCUITBREAKERS_CLUSTER_ENVOYPROXY_MAXCONNECTIONS = "thresholds.circuitbreakers.cluster.envoyproxy.io/max-connections"
const ANNOTATIONS_THRESHOLDS_CIRCUITBREAKERS_CLUSTER_ENVOYPROXY_MAXPENDINGREQUESTS = "thresholds.circuitbreakers.cluster.envoyproxy.io/max-pending-requests"
const ANNOTATIONS_THRESHOLDS_CIRCUITBREAKERS_CLUSTER_ENVOYPROXY_MAXREQUESTS = "thresholds.circuitbreakers.cluster.envoyproxy.io/max-requests"
const ANNOTATIONS_THRESHOLDS_CIRCUITBREAKERS_CLUSTER_ENVOYPROXY_MAXRETRIES = "thresholds.circuitbreakers.cluster.envoyproxy.io/max-retries"

func getAnnotation(ingress *v1beta1.Ingress, key string, defaultValue string) string {
	annotationValue := ingress.ObjectMeta.Annotations[key]
	if annotationValue == "" {
		return defaultValue
	}
	return annotationValue
}
