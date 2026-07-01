package podresources

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync/atomic"

	"google.golang.org/grpc"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"
)

// Server implements the kubelet podresources v1 PodResourcesLister (List only), answering
// from an atomically-swapped snapshot. Unlisted RPCs return Unimplemented via the embed.
type Server struct {
	podresourcesv1.UnimplementedPodResourcesListerServer
	socket string
	grpc   *grpc.Server
	snap   atomic.Pointer[[]*podresourcesv1.PodResources]
}

func NewServer(socket string) *Server {
	return &Server{socket: socket}
}

func (s *Server) SetSnapshot(pods []*podresourcesv1.PodResources) {
	s.snap.Store(&pods)
}

func (s *Server) List(_ context.Context, _ *podresourcesv1.ListPodResourcesRequest) (*podresourcesv1.ListPodResourcesResponse, error) {
	p := s.snap.Load()
	if p == nil {
		return &podresourcesv1.ListPodResourcesResponse{}, nil
	}
	return &podresourcesv1.ListPodResourcesResponse{PodResources: *p}, nil
}

// Start removes any stale socket, listens, registers, and serves in a goroutine.
func (s *Server) Start() error {
	if err := s.cleanup(); err != nil {
		return err
	}
	lis, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.socket, err)
	}
	s.grpc = grpc.NewServer()
	podresourcesv1.RegisterPodResourcesListerServer(s.grpc, s)
	go func() {
		if err := s.grpc.Serve(lis); err != nil {
			log.Printf("podresources server stopped: %v", err)
		}
	}()
	return nil
}

func (s *Server) Stop() {
	if s.grpc != nil {
		s.grpc.Stop()
		s.grpc = nil
	}
	_ = s.cleanup()
}

func (s *Server) cleanup() error {
	if err := os.Remove(s.socket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket %s: %w", s.socket, err)
	}
	return nil
}

func (s *Server) snapshot() []*podresourcesv1.PodResources {
	p := s.snap.Load()
	if p == nil {
		return nil
	}
	return *p
}
