package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

type nvidiaSmiArgs struct {
	GpuProduct  string
	GpuUsedMem  float32
	GpuTotalMem int
	GpuUtil     int
	GpuIdx      int
	ProcessName string
}

type config struct {
	Debug bool
}

var conf config = config{}

// main is the entry point for the application.
func main() {
	flag.BoolVar(&conf.Debug, "debug", false, "enable debug mode")
	flag.Parse()

	if conf.Debug {
		fmt.Println("Debug mode enabled")
	}

	err := os.Setenv(constants.EnvTopologyCmNamespace, "gpu-operator")
	if err != nil {
		panic(err)
	}
	err = os.Setenv(constants.EnvTopologyCmName, "topology")
	if err != nil {
		panic(err)
	}

	if conf.Debug {
		fmt.Printf("Set topology configmap namespace: %s\n", constants.EnvTopologyCmNamespace)
		fmt.Printf("Set topology configmap name: %s\n", constants.EnvTopologyCmName)
	}

	args := getNvidiaSmiArgs()

	printArgs(args)
}

// getTopologyFromEnv retrieves topology from GPU_TOPOLOGY_JSON environment variable
func getTopologyFromEnv() (*topology.NodeTopology, error) {
	topologyJSON := os.Getenv("GPU_TOPOLOGY_JSON")
	if topologyJSON == "" {
		return nil, fmt.Errorf("GPU_TOPOLOGY_JSON not set")
	}

	var nodeTopology topology.NodeTopology
	err := json.Unmarshal([]byte(topologyJSON), &nodeTopology)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GPU_TOPOLOGY_JSON: %w", err)
	}

	return &nodeTopology, nil
}

// getTopologyFromHTTP retrieves topology from HTTP topology server
func getTopologyFromHTTP(nodeName string) (*topology.NodeTopology, error) {
	topologyUrl := "http://topology-server.gpu-operator/topology/nodes/" + nodeName
	if conf.Debug {
		fmt.Printf("Requesting topology from: %s\n", topologyUrl)
	}
	resp, err := http.Get(topologyUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to get topology from HTTP: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Ignore close errors on response body
		}
	}()

	var nodeTopology topology.NodeTopology
	err = json.NewDecoder(resp.Body).Decode(&nodeTopology)
	if err != nil {
		return nil, fmt.Errorf("failed to decode topology response: %w", err)
	}

	return &nodeTopology, nil
}

func getNvidiaSmiArgs() (args nvidiaSmiArgs) {
	nodeName := os.Getenv(constants.EnvNodeName)
	if conf.Debug {
		fmt.Printf("Node name: %s\n", nodeName)
	}

	// Try to get topology from environment variable first
	var nodeTopology *topology.NodeTopology
	var err error

	nodeTopology, err = getTopologyFromEnv()
	if err != nil {
		if conf.Debug {
			fmt.Printf("Failed to get topology from env: %v, falling back to HTTP\n", err)
		}
		// Fallback to HTTP topology server
		nodeTopology, err = getTopologyFromHTTP(nodeName)
		if err != nil {
			panic(fmt.Errorf("failed to get topology from both env and HTTP: %w", err))
		}
		if conf.Debug {
			fmt.Printf("Successfully loaded topology from HTTP server\n")
		}
	} else {
		if conf.Debug {
			fmt.Printf("Successfully loaded topology from GPU_TOPOLOGY_JSON env var\n")
		}
	}

	if conf.Debug {
		fmt.Printf("Received topology: %+v\n", nodeTopology)
	}

	args.GpuProduct = nodeTopology.GpuProduct

	gpuPortion := 1.0
	// READ RUNAI_NUM_OF_GPUS float env variable
	numOfGpus := os.Getenv("RUNAI_NUM_OF_GPUS")
	if numOfGpus != "" {
		gpuPortion, err = strconv.ParseFloat(numOfGpus, 32)
		if err != nil {
			panic(err)
		}
		if conf.Debug {
			fmt.Printf("GPU portion from RUNAI_NUM_OF_GPUS: %f\n", gpuPortion)
		}
	}
	args.GpuTotalMem = int(float64(nodeTopology.GpuMemory) * gpuPortion)

	var gpuIdx int
	currentPodName := os.Getenv("HOSTNAME")
	currentPodUuid := os.Getenv("POD_UUID")
	if conf.Debug {
		fmt.Printf("Current pod name: %s, UUID: %s\n", currentPodName, currentPodUuid)
	}

	for idx, gpu := range nodeTopology.Gpus {
		if gpu.Status.AllocatedBy.Pod == currentPodName {
			gpuIdx = idx
			if conf.Debug {
				fmt.Printf("Found GPU %d allocated to pod %s\n", idx, currentPodName)
			}
			break
		}

		for podUuid := range gpu.Status.PodGpuUsageStatus {
			if string(podUuid) == currentPodUuid {
				gpuIdx = idx
				if conf.Debug {
					fmt.Printf("Found GPU %d used by pod UUID %s\n", idx, currentPodUuid)
				}
				break
			}
		}
	}

	args.GpuIdx = gpuIdx
	if len(nodeTopology.Gpus) > 0 && gpuIdx < len(nodeTopology.Gpus) {
		args.GpuUsedMem = float32(nodeTopology.Gpus[gpuIdx].Status.PodGpuUsageStatus.FbUsed(nodeTopology.GpuMemory)) * float32(gpuPortion)
		args.GpuUtil = nodeTopology.Gpus[gpuIdx].Status.PodGpuUsageStatus.Utilization()
	}

	if conf.Debug {
		fmt.Printf("GPU stats - Index: %d, Used Memory: %f, Utilization: %d%%\n", args.GpuIdx, args.GpuUsedMem, args.GpuUtil)
	}

	// Read /proc/1/cmdline to get the process name
	cmdlineFile, err := os.Open("/proc/1/cmdline")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := cmdlineFile.Close(); err != nil {
			fmt.Printf("Error closing cmdline file: %v\n", err)
		}
	}()

	// Read the file
	cmdlineBytes := make([]byte, 50)
	_, err = cmdlineFile.Read(cmdlineBytes)
	if err != nil {
		panic(err)
	}

	args.ProcessName = string(bytes.Trim(cmdlineBytes, "\x00"))
	if conf.Debug {
		fmt.Printf("Process name from /proc/1/cmdline: %s\n", args.ProcessName)
	}

	return args
}

func printArgs(args nvidiaSmiArgs) {
	// Example:
	//
	// Wed Jun 29 14:19:35 2022
	// +-----------------------------------------------------------------------------+
	// | NVIDIA-SMI 470.129.06   Driver Version: 470.129.06   CUDA Version: 11.4     |
	// |-------------------------------+----------------------+----------------------+
	// | GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
	// | Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
	// |                               |                      |               MIG M. |
	// |===============================+======================+======================|
	// |   0  Tesla T4            Off  | 00000001:00:00.0 Off |                  Off |
	// | N/A   33C    P8    11W /  70W |      4MiB / 16127MiB |      0%      Default |
	// |                               |                      |                  N/A |
	// +-------------------------------+----------------------+----------------------+

	// +-----------------------------------------------------------------------------+
	// | Processes:                                                                  |
	// |  GPU   GI   CI        PID   Type   Process name                  GPU Memory |
	// |        ID   ID                                                   Usage      |
	// |=============================================================================|
	// |    0   N/A  N/A       964      G   /usr/lib/xorg/Xorg                  4MiB |
	// +-----------------------------------------------------------------------------+

	if conf.Debug {
		fmt.Println("Printing nvidia-smi output")
	}

	// Print date
	fmt.Println(time.Now().Format(time.ANSIC))
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetTitle("NVIDIA-SMI 470.129.06   Driver Version: 470.129.06   CUDA Version: 11.4")
	t.AppendSeparator()
	t.AppendRow(table.Row{"GPU  Name        Persistence-M", "Bus-Id        Disp.A", "Volatile Uncorr. ECC"})
	t.AppendRow(table.Row{"Fan  Temp  Perf  Pwr:Usage/Cap", "        Memory-Usage", "GPU-Util  Compute M."})
	t.AppendRow(table.Row{"", "", "              MIG M."})
	t.AppendSeparator()
	t.AppendRow(table.Row{fmt.Sprintf("%s  %s%s", sizeString(strconv.Itoa(args.GpuIdx), 3, true), sizeString(args.GpuProduct, 12, false), sizeString("Off", 13, true)), fmt.Sprintf("%s %s", sizeString("00000001:00:00.0", 16, false), sizeString("Off", 3, true)), sizeString("Off", 20, true)})
	t.AppendRow(table.Row{"N/A   33C    P8    11W /  70W", sizeString(fmt.Sprintf("%dMiB / %dMiB", int(args.GpuUsedMem), args.GpuTotalMem), 20, true), fmt.Sprintf("%s %s", sizeString(strconv.Itoa(args.GpuUtil)+"%", 8, true), sizeString("Default", 11, true))})
	t.AppendRow(table.Row{"", "", sizeString("N/A", 20, true)})
	t.AppendSeparator()
	t.Render()

	fmt.Printf("\n")

	t = table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendRow(table.Row{"Processes:"})
	t.AppendRow(table.Row{" GPU   GI   CI        PID   Type   Process name                  GPU Memory "})
	t.AppendRow(table.Row{"       ID   ID                                                   Usage      "})
	t.AppendSeparator()
	t.AppendRow(table.Row{fmt.Sprintf(" %s   %s%s%s%s   %s %s", sizeString(strconv.Itoa(args.GpuIdx), 3, true), sizeString("N/A", 5, false), sizeString("N/A", 10, false), sizeString(strconv.Itoa(os.Getpid()), 6, false), sizeString("G", 4, true), sizeString(args.ProcessName, 29, false), sizeString(fmt.Sprintf("%dMiB", int(args.GpuUsedMem)), 11, true))})
	t.Render()
}

func sizeString(str string, size int, alignRight bool) string {
	if len(str) < size {
		if alignRight {
			str = strings.Repeat(" ", size-len(str)) + str
		} else {
			str = str + strings.Repeat(" ", size-len(str))
		}
	}

	if len(str) > size {
		str = str[:size-2] + ".."
	}

	return str[:size]
}
