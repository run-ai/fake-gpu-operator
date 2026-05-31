package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

func getNvidiaSmiArgs() []nvidiaSmiArgs {
	nodeName := os.Getenv(constants.EnvNodeName)
	if conf.Debug {
		fmt.Printf("Node name: %s\n", nodeName)
	}

	topologyUrl := "http://topology-server.gpu-operator/topology/nodes/" + nodeName
	if conf.Debug {
		fmt.Printf("Requesting topology from: %s\n", topologyUrl)
	}
	resp, err := http.Get(topologyUrl)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("Error closing response body: %v\n", err)
		}
	}()

	// Guard against non-JSON responses (e.g. an empty NODE_NAME yields a plain-text
	// error from topology-server) so we fail with a clear message instead of a
	// cryptic JSON decode panic.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		panic(fmt.Sprintf("topology-server returned %d for %s: %s", resp.StatusCode, topologyUrl, strings.TrimSpace(string(body))))
	}

	var nodeTopology topology.NodeTopology
	err = json.NewDecoder(resp.Body).Decode(&nodeTopology)
	if err != nil {
		panic(err)
	}
	if conf.Debug {
		fmt.Printf("Received topology: %+v\n", nodeTopology)
	}

	gpuPortion := 1.0
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
	gpuTotalMem := int(float64(nodeTopology.GpuMemory) * gpuPortion)

	currentPodName := os.Getenv("HOSTNAME")
	currentPodUuid := os.Getenv("POD_UUID")
	if conf.Debug {
		fmt.Printf("Current pod name: %s, UUID: %s\n", currentPodName, currentPodUuid)
	}

	processName := readProcessName()

	var allArgs []nvidiaSmiArgs
	for idx, gpu := range nodeTopology.Gpus {
		matched := false
		if gpu.Status.AllocatedBy.Pod == currentPodName {
			matched = true
		} else {
			for podUuid := range gpu.Status.PodGpuUsageStatus {
				if string(podUuid) == currentPodUuid {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		if conf.Debug {
			fmt.Printf("Found GPU %d allocated to pod %s\n", idx, currentPodName)
		}
		allArgs = append(allArgs, nvidiaSmiArgs{
			GpuProduct:  nodeTopology.GpuProduct,
			GpuTotalMem: gpuTotalMem,
			GpuUsedMem:  float32(gpu.Status.PodGpuUsageStatus.FbUsed(nodeTopology.GpuMemory)) * float32(gpuPortion),
			GpuUtil:     gpu.Status.PodGpuUsageStatus.Utilization(),
			GpuIdx:      idx,
			ProcessName: processName,
		})
	}

	if len(allArgs) == 0 {
		allArgs = append(allArgs, nvidiaSmiArgs{
			GpuProduct:  nodeTopology.GpuProduct,
			GpuTotalMem: gpuTotalMem,
			ProcessName: processName,
		})
	}

	return allArgs
}

func readProcessName() string {
	cmdlineFile, err := os.Open("/proc/1/cmdline")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := cmdlineFile.Close(); err != nil {
			fmt.Printf("Error closing cmdline file: %v\n", err)
		}
	}()

	cmdlineBytes := make([]byte, 50)
	_, err = cmdlineFile.Read(cmdlineBytes)
	if err != nil {
		panic(err)
	}

	return string(bytes.Trim(cmdlineBytes, "\x00"))
}

func printArgs(allArgs []nvidiaSmiArgs) {
	if conf.Debug {
		fmt.Println("Printing nvidia-smi output")
	}

	fmt.Println(time.Now().Format(time.ANSIC))
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetTitle("NVIDIA-SMI 470.129.06   Driver Version: 470.129.06   CUDA Version: 11.4")
	t.AppendSeparator()
	t.AppendRow(table.Row{"GPU  Name        Persistence-M", "Bus-Id        Disp.A", "Volatile Uncorr. ECC"})
	t.AppendRow(table.Row{"Fan  Temp  Perf  Pwr:Usage/Cap", "        Memory-Usage", "GPU-Util  Compute M."})
	t.AppendRow(table.Row{"", "", "              MIG M."})
	t.AppendSeparator()
	for _, args := range allArgs {
		t.AppendRow(table.Row{fmt.Sprintf("%s  %s%s", sizeString(strconv.Itoa(args.GpuIdx), 3, true), sizeString(args.GpuProduct, 12, false), sizeString("Off", 13, true)), fmt.Sprintf("%s %s", sizeString("00000001:00:00.0", 16, false), sizeString("Off", 3, true)), sizeString("Off", 20, true)})
		t.AppendRow(table.Row{"N/A   33C    P8    11W /  70W", sizeString(fmt.Sprintf("%dMiB / %dMiB", int(args.GpuUsedMem), args.GpuTotalMem), 20, true), fmt.Sprintf("%s %s", sizeString(strconv.Itoa(args.GpuUtil)+"%", 8, true), sizeString("Default", 11, true))})
		t.AppendRow(table.Row{"", "", sizeString("N/A", 20, true)})
		t.AppendSeparator()
	}
	t.Render()

	fmt.Printf("\n")

	t = table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendRow(table.Row{"Processes:"})
	t.AppendRow(table.Row{" GPU   GI   CI        PID   Type   Process name                  GPU Memory "})
	t.AppendRow(table.Row{"       ID   ID                                                   Usage      "})
	t.AppendSeparator()
	for _, args := range allArgs {
		t.AppendRow(table.Row{fmt.Sprintf(" %s   %s%s%s%s   %s %s", sizeString(strconv.Itoa(args.GpuIdx), 3, true), sizeString("N/A", 5, false), sizeString("N/A", 10, false), sizeString(strconv.Itoa(os.Getpid()), 6, false), sizeString("G", 4, true), sizeString(args.ProcessName, 29, false), sizeString(fmt.Sprintf("%dMiB", int(args.GpuUsedMem)), 11, true))})
	}
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
