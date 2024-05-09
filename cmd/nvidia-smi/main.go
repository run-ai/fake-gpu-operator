package main

import (
	"bytes"
	"encoding/json"
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

// main is the entry point for the application.
func main() {
	os.Setenv(constants.EnvTopologyCmNamespace, "gpu-operator")
	os.Setenv(constants.EnvTopologyCmName, "topology")

	args := getNvidiaSmiArgs()

	printArgs(args)
}

func getNvidiaSmiArgs() (args nvidiaSmiArgs) {
	nodeName := os.Getenv(constants.EnvNodeName)

	// Send http request to topology-server to get the topology
	resp, err := http.Get("http://topology-server.gpu-operator/topology/nodes/" + nodeName)
	if err != nil {
		panic(err)
	}

	// Parse the response
	var nodeTopology topology.NodeTopology
	err = json.NewDecoder(resp.Body).Decode(&nodeTopology)
	if err != nil {
		panic(err)
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
	}
	args.GpuTotalMem = int(float64(nodeTopology.GpuMemory) * gpuPortion)

	var gpuIdx int
	currentPodName := os.Getenv("HOSTNAME")
	currentPodUuid := os.Getenv("POD_UUID")
	for idx, gpu := range nodeTopology.Gpus {
		if gpu.Status.AllocatedBy.Pod == currentPodName {
			gpuIdx = idx
			break
		}

		for podUuid := range gpu.Status.PodGpuUsageStatus {
			if string(podUuid) == currentPodUuid {
				gpuIdx = idx
				break
			}
		}
	}

	args.GpuIdx = gpuIdx
	args.GpuUsedMem = float32(nodeTopology.Gpus[gpuIdx].Status.PodGpuUsageStatus.FbUsed(nodeTopology.GpuMemory)) * float32(gpuPortion)
	args.GpuUtil = nodeTopology.Gpus[gpuIdx].Status.PodGpuUsageStatus.Utilization()

	// Read /proc/1/cmdline to get the process name
	cmdlineFile, err := os.Open("/proc/1/cmdline")
	if err != nil {
		panic(err)
	}
	defer cmdlineFile.Close()

	// Read the file
	cmdlineBytes := make([]byte, 50)
	_, err = cmdlineFile.Read(cmdlineBytes)
	if err != nil {
		panic(err)
	}

	args.ProcessName = string(bytes.Trim(cmdlineBytes, "\x00"))

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
