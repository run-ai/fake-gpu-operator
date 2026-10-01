package topology

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// promServer answers instant queries with a single sample of the given value
// and records the query it received.
func promServer(t *testing.T, value string, gotQuery *string) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotQuery = r.URL.Query().Get("query")
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700000000,"%s"]}]}}`, value)
	}))
	t.Cleanup(server.Close)

	previous := prometheusBaseURL
	InitPrometheusConfig(server.URL)
	t.Cleanup(func() { InitPrometheusConfig(previous) })
}

func TestKnativeUtilization_SumsRequestRateForPod(t *testing.T) {
	var query string
	promServer(t, "3", &query)

	m := PodGpuUsageStatusMap{}
	assert.Equal(t, 3, m.knativeUtilization("pod-uid"))
	assert.Equal(t,
		`sum(rate(revision_app_request_count[1m]) * on(pod) group_left(uid) kube_pod_info{uid="pod-uid"})`,
		query)
}

func TestKnativeUtilization_RoundsFloatingPointRate(t *testing.T) {
	var query string
	promServer(t, "2.9999999999999996", &query)

	m := PodGpuUsageStatusMap{}
	assert.Equal(t, 3, m.knativeUtilization("pod-uid"))
}
