package topology

import (
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"

	"github.com/tidwall/gjson"
)

func (m *PodGpuUsageStatusMap) Utilization() int {
	var sum int
	for k, v := range *m {
		if v.IsInferencePod {
			sum += m.knativeUtilization(string(k))
		} else {
			sum += v.Utilization.Random()
		}
	}

	return int(math.Min(100, float64(sum)))
}

func (m *PodGpuUsageStatusMap) FbUsed(fbTotal int) int {
	var sum int
	for _, v := range *m {
		sum += v.FbUsed
	}

	return int(math.Min(float64(fbTotal), float64(sum)))
}

func (m *PodGpuUsageStatusMap) knativeUtilization(uid string) int {
	query := fmt.Sprintf("(rate(revision_app_request_count[1m]) + on(pod) group_left(uid) kube_pod_info{uid=\"%s\"})", uid)
	params := url.Values{}
	params.Set("query", query)
	res, err := http.Get("http://runai-cluster-kube-prometh-prometheus.monitoring:9090/api/v1/query?" + params.Encode())
	if err != nil {
		log.Printf("Error: %v\n", err)
		return 0
	}
	defer res.Body.Close()

	// ReadAll body
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return 0
	}

	val := gjson.Get(string(body), "data.result.#.value").Array()
	if len(val) == 0 {
		return 0
	}

	val = val[0].Array()
	if len(val) == 0 {
		return 0
	}

	return int(val[1].Float())
}
