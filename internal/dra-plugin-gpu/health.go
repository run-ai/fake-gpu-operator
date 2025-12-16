package dra_plugin_gpu

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"path"
	"strconv"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	drapb "k8s.io/kubelet/pkg/apis/dra/v1"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
)

type healthcheck struct {
	grpc_health_v1.UnimplementedHealthServer

	server *grpc.Server
	wg     sync.WaitGroup

	regClient registerapi.RegistrationClient
	draClient drapb.DRAPluginClient
}

func StartHealthcheck(_ context.Context, config *Config) (*healthcheck, error) {
	port := config.Flags.HealthcheckPort
	if port < 0 {
		return nil, nil
	}

	addr := net.JoinHostPort("", strconv.Itoa(port))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen for healthcheck service at %s: %w", addr, err)
	}

	regSockPath := (&url.URL{
		Scheme: "unix",
		Path:   path.Join(config.Flags.KubeletRegistrarDirectoryPath, DriverName+"-reg.sock"),
	}).String()
	log.Printf("Connecting to registration socket: %s", regSockPath)
	regConn, err := grpc.NewClient(
		regSockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to registration socket: %w", err)
	}

	draSockPath := (&url.URL{
		Scheme: "unix",
		Path:   path.Join(config.DriverPluginPath(), "dra.sock"),
	}).String()
	log.Printf("Connecting to DRA socket: %s", draSockPath)
	draConn, err := grpc.NewClient(
		draSockPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to DRA socket: %w", err)
	}

	server := grpc.NewServer()
	healthcheck := &healthcheck{
		server:    server,
		regClient: registerapi.NewRegistrationClient(regConn),
		draClient: drapb.NewDRAPluginClient(draConn),
	}
	grpc_health_v1.RegisterHealthServer(server, healthcheck)

	healthcheck.wg.Add(1)
	go func() {
		defer healthcheck.wg.Done()
		log.Printf("Starting healthcheck service at %s", lis.Addr().String())
		if err := server.Serve(lis); err != nil {
			log.Printf("Failed to serve healthcheck: %v", err)
		}
	}()

	return healthcheck, nil
}

func (h *healthcheck) Stop() {
	if h.server != nil {
		log.Printf("Stopping healthcheck service")
		h.server.GracefulStop()
	}
	h.wg.Wait()
}

// Check implements [grpc_health_v1.HealthServer].
func (h *healthcheck) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	knownServices := map[string]struct{}{"": {}, "liveness": {}}
	if _, known := knownServices[req.GetService()]; !known {
		return nil, status.Error(codes.NotFound, "unknown service")
	}

	resp := &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
	}

	if _, err := h.regClient.GetInfo(ctx, &registerapi.InfoRequest{}); err != nil {
		log.Printf("Healthcheck: GetInfo failed: %v", err)
		return resp, nil
	}

	if _, err := h.draClient.NodePrepareResources(ctx, &drapb.NodePrepareResourcesRequest{}); err != nil {
		log.Printf("Healthcheck: NodePrepareResources failed: %v", err)
		return resp, nil
	}

	resp.Status = grpc_health_v1.HealthCheckResponse_SERVING
	return resp, nil
}
