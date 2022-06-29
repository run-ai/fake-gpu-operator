package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

var nvidiaSmiOutputTemplate = `
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 396.15                 Driver Version: 396.15                    |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
|===============================+======================+======================|
|   {{ .GpuIdx }}{{ .Space1 }}{{ .GpuProduct }}  Off  | 00000002:01:00.0 Off |                    0 |
| N/A   30C    P0    38W / 300W |{{ .Space2 }}{{ .GpuUsedMem }}MiB / {{ .GpuTotalMem }}MiB{{ .Space3 }}|{{ .Space4 }}{{ .GpuUtil }}%      Default |
+-------------------------------+----------------------+----------------------+
                                                                               
+-----------------------------------------------------------------------------+
| Processes:                                                       GPU Memory |
|  GPU       PID   Type   Process name                             Usage      |
|=============================================================================|
|    0    124097      C   {{ .ProcessName }}{{ .Space5 }}{{ .GpuUsedMem }}MiB{{ .Space6 }}|
+-----------------------------------------------------------------------------+
`

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
	os.Setenv("TOPOLOGY_CM_NAMESPACE", "gpu-operator")
	os.Setenv("TOPOLOGY_CM_NAME", "topology")

	args := getNvidiaSmiArgs()

	log.Printf("%+v", args)

	printArgs(args)
	args = rightSizeArgs(args)

	// t := template.New("nvidia-smi")

	// t, err = t.Parse(nvidiaSmiOutputTemplate)
	// if err != nil {
	// 	panic(err)
	// }

	// err = t.Execute(os.Stdout, args)
	// if err != nil {
	// 	panic(err)
	// }
}

func getNvidiaSmiArgs() (args nvidiaSmiArgs) {
	nodeName := os.Getenv("NODE_NAME")

	// Send http request to topology-server to get the topology
	resp, err := http.Get("http://topology-server.gpu-operator/")
	if err != nil {
		panic(err)
	}

	// Parse the response
	var clusterTopology topology.ClusterTopology
	err = json.NewDecoder(resp.Body).Decode(&clusterTopology)
	if err != nil {
		panic(err)
	}

	nodeTopology, ok := clusterTopology.Nodes[nodeName]
	if !ok {
		panic("nodeTopology not found")
	}

	args.GpuProduct = nodeTopology.GpuProduct
	args.GpuTotalMem = nodeTopology.GpuMemory

	var gpuIdx int
	if os.Getenv("NVIDIA_VISIBLE_DEVICES") == "" {
		// Whole GPU is used
		podName := os.Getenv("HOSTNAME")
		// Search clusterTopology for the podName
		for _, node := range clusterTopology.Nodes {
			for idx, gpu := range node.Gpus {
				if gpu.Metrics.Metadata.Pod == podName {
					gpuIdx = idx
				}
			}
		}
	} else {
		// Shared GPU is used
		gpuIdxStr := os.Getenv("NVIDIA_VISIBLE_DEVICES")
		gpuIdx, err = strconv.Atoi(gpuIdxStr)
		if err != nil {
			panic(err)
		}
	}

	args.GpuIdx = gpuIdx
	args.GpuUsedMem = float32(nodeTopology.Gpus[gpuIdx].Metrics.Status.FbUsed)
	args.GpuUtil = nodeTopology.Gpus[gpuIdx].Metrics.Status.Utilization

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

	args.ProcessName = string(cmdlineBytes)

	return args
}

// // simple table with zero customizations
// tw := NewWriter()
// // append a header row
// tw.AppendHeader(Row{"#", "First Name", "Last Name", "Salary"})
// // append some data rows
// tw.AppendRows([]Row{
// 	{1, "Arya", "Stark", 3000},
// 	{20, "Jon", "Snow", 2000, "You know nothing, Jon Snow!"},
// 	{300, "Tyrion", "Lannister", 5000},
// })
// // append a footer row
// tw.AppendFooter(Row{"", "", "Total", 10000})
// // render it
// fmt.Printf("Table without any customizations:\n%s", tw.Render())

func printArgs(args nvidiaSmiArgs) {
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

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetTitle("NVIDIA-SMI 470.129.06   Driver Version: 470.129.06   CUDA Version: 11.4")
	t.AppendSeparator()
	t.AppendRow(table.Row{"GPU  Name        Persistence-M", "Bus-Id        Disp.A", "Volatile   Uncorr. ECC"})
	t.AppendRow(table.Row{"Fan  Temp  Perf  Pwr:Usage/Cap", "        Memory-Usage", "GPU-Util   Compute M."})
	t.AppendRow(table.Row{"", "", "               MIG M."})
	t.AppendSeparator()
	t.AppendRow(table.Row{fmt.Sprintf(" %d   %s           Off", args.GpuIdx, args.GpuProduct), "00000001:00:00.0 Off", " 			   Off"})
	t.AppendRow(table.Row{" N/A   33C    P8    11W /  70W", fmt.Sprintf("  %dMiB / %dMiB", args.GpuTotalMem, args.GpuTotalMem), fmt.Sprintf("  %d%%        Default", args.GpuUtil)})
	t.AppendRow(table.Row{"", "", "               N/A"})
	t.AppendSeparator()
	t.Render()

	fmt.Printf("\n")

	t = table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendRow(table.Row{"Processes:"})
	t.AppendRow(table.Row{"GPU   GI   CI            PID   Type   Process name                  GPU Memory"})
	t.AppendRow(table.Row{"      ID   ID                                                       Usage      "})
	t.AppendSeparator()
	t.AppendRow(table.Row{fmt.Sprintf(" %d   N/A  N/A             1      G    %s 				%dMiB", args.GpuIdx, args.ProcessName, int(args.GpuUsedMem))})
	t.Render()
}

func rightSizeArgs(args nvidiaSmiArgs) nvidiaSmiArgs {
	if len(args.GpuProduct) > 18 {
		args.GpuProduct = args.GpuProduct[:15]
	}

	return args
}
