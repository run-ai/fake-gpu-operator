package kubeclient

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// The apiserver closes every watch after a timeout. The client must re-open it
// (retrying failures) and keep delivering updates, not spin on the closed channel.
func TestWatchConfigMapSurvivesClosedWatch(t *testing.T) {
	defaultRetryInterval := watchRetryInterval
	watchRetryInterval = time.Millisecond
	t.Cleanup(func() { watchRetryInterval = defaultRetryInterval })

	clientset := fake.NewClientset()
	watchers := make(chan *watch.FakeWatcher, 3)
	calls := 0
	clientset.PrependWatchReactor("configmaps", func(k8stesting.Action) (bool, watch.Interface, error) {
		calls++
		if calls == 2 {
			return true, nil, errors.New("apiserver unavailable")
		}
		w := watch.NewFake()
		watchers <- w
		return true, w, nil
	})

	stop := make(chan struct{})
	client := &KubeClient{ClientSet: clientset, stopChan: stop}
	cms, err := client.WatchConfigMap("ns", "topology")
	require.NoError(t, err)

	first := receive(t, watchers)
	first.Stop()

	second := receive(t, watchers)
	second.Modify(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "topology"}})
	require.Equal(t, "topology", receive(t, cms).Name)

	close(stop)
	require.Eventually(t, second.IsStopped, time.Second, time.Millisecond)
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting on channel")
		var zero T
		return zero
	}
}
