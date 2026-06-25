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
	GpuProduct    string
	DriverVersion string
	CudaVersion   string
	GpuUsedMem    float32
	GpuTotalMem   int
	GpuUtil       int
	GpuIdx        int
	ProcessName   string
}

// Fallback versions for pools whose config carries no driver/CUDA version
// (e.g. old-format topology without a GPU profile).
const (
	defaultDriverVersion = "470.129.06"
	defaultCudaVersion   = "11.4"
)

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

	args, summary, errs := getNvidiaSmiArgs()

	if len(args) == 0 {
		fmt.Println(summary)
		printErrors(errs)
		os.Exit(1)
	}

	printArgs(args)
	printErrors(errs)
}

func printErrors(errs []error) {
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, err)
	}
}

// getNvidiaSmiArgs returns the per-GPU rows to render. When no devices can be
// obtained it returns an empty slice plus a user-facing summary line describing
// the failure (printed to stdout), with the underlying causes in errs (stderr).
func getNvidiaSmiArgs() (args []nvidiaSmiArgs, summary string, errs []error) {

	nodeName := os.Getenv(constants.EnvNodeName)
	if conf.Debug {
		fmt.Printf("Node name: %s\n", nodeName)
	}

	topologyUrl := "http://topology-server.gpu-operator/topology/nodes/" + nodeName
	if conf.Debug {
		fmt.Printf("Requesting topology from: %s\n", topologyUrl)
	}
	// Topology is required to render any device, so fetch/status/decode failures
	// are fatal: accumulate the error and return no devices.
	resp, err := http.Get(topologyUrl)
	if err != nil {
		return nil, "NVIDIA-SMI has failed because it couldn't communicate with the topology server.", append(errs, fmt.Errorf("fetching topology from %s: %w", topologyUrl, err))
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing topology response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "No devices were found", append(errs, fmt.Errorf("topology server %s returned %s: %s", topologyUrl, resp.Status, strings.TrimSpace(string(body))))
	}

	var nodeTopology topology.NodeTopology
	if err := json.NewDecoder(resp.Body).Decode(&nodeTopology); err != nil {
		return nil, "Failed to parse devices data", append(errs, fmt.Errorf("decoding topology from %s: %w", topologyUrl, err))
	}
	if conf.Debug {
		fmt.Printf("Received topology: %+v\n", nodeTopology)
	}

	// gpuPortion only affects reported memory; a bad RUNAI_NUM_OF_GPUS is
	// non-fatal — keep the default and still render the table.
	gpuPortion := 1.0
	numOfGpus := os.Getenv("RUNAI_NUM_OF_GPUS")
	if numOfGpus != "" {
		if parsed, err := strconv.ParseFloat(numOfGpus, 32); err != nil {
			errs = append(errs, fmt.Errorf("parsing RUNAI_NUM_OF_GPUS %q: %w", numOfGpus, err))
		} else {
			gpuPortion = parsed
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

	// A missing process name only blanks one table column — non-fatal.
	processName, err := readProcessName()
	if err != nil {
		errs = append(errs, err)
	}

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
			GpuProduct:    nodeTopology.GpuProduct,
			DriverVersion: nodeTopology.DriverVersion,
			CudaVersion:   nodeTopology.CudaVersion,
			GpuTotalMem:   gpuTotalMem,
			GpuUsedMem:    float32(gpu.Status.PodGpuUsageStatus.FbUsed(nodeTopology.GpuMemory)) * float32(gpuPortion),
			GpuUtil:       gpu.Status.PodGpuUsageStatus.Utilization(),
			GpuIdx:        idx,
			ProcessName:   processName,
		})
	}

	if len(allArgs) == 0 {
		allArgs = append(allArgs, nvidiaSmiArgs{
			GpuProduct:    nodeTopology.GpuProduct,
			DriverVersion: nodeTopology.DriverVersion,
			CudaVersion:   nodeTopology.CudaVersion,
			GpuTotalMem:   gpuTotalMem,
			ProcessName:   processName,
		})
	}

	return allArgs, "", errs
}

func readProcessName() (string, error) {
	cmdlineFile, err := os.Open("/proc/1/cmdline")
	if err != nil {
		return "", fmt.Errorf("opening /proc/1/cmdline: %w", err)
	}
	defer func() {
		if err := cmdlineFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing cmdline file: %v\n", err)
		}
	}()

	cmdlineBytes := make([]byte, 50)
	if _, err := cmdlineFile.Read(cmdlineBytes); err != nil {
		return "", fmt.Errorf("reading /proc/1/cmdline: %w", err)
	}

	return string(bytes.Trim(cmdlineBytes, "\x00")), nil
}

func printArgs(allArgs []nvidiaSmiArgs) {
	if conf.Debug {
		fmt.Println("Printing nvidia-smi output")
	}

	driverVersion := defaultDriverVersion
	if allArgs[0].DriverVersion != "" {
		driverVersion = allArgs[0].DriverVersion
	}
	cudaVersion := defaultCudaVersion
	if allArgs[0].CudaVersion != "" {
		cudaVersion = allArgs[0].CudaVersion
	}

	fmt.Println(time.Now().Format(time.ANSIC))
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetTitle(fmt.Sprintf("NVIDIA-SMI %s   Driver Version: %s   CUDA Version: %s", driverVersion, driverVersion, cudaVersion))
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
