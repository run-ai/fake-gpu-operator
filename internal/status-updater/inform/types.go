package inform

// import kubernetes core
import (
	v1 "k8s.io/api/core/v1"
)

type PodEvent struct {
	Pod       *v1.Pod
	EventType EventType
}

type EventType string

const (
	ADD    EventType = "ADD"
	DELETE EventType = "DELETE"
)
