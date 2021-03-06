package main

import (
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"

	"github.com/childe/gohangout/filter"
	"github.com/childe/gohangout/input"
	"github.com/childe/gohangout/output"
	"github.com/golang/glog"
	"github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary
var options = &struct {
	config     string
	pprof      bool
	pprofAddr  string
	cpuprofile string
	memprofile string
}{}

func init() {
	flag.StringVar(&options.config, "config", options.config, "path to configuration file or directory")

	flag.BoolVar(&options.pprof, "pprof", false, "if pprof")
	flag.StringVar(&options.pprofAddr, "pprof-address", "127.0.0.1:8899", "default: 127.0.0.1:8899")
	flag.StringVar(&options.cpuprofile, "cpuprofile", "", "write cpu profile to `file`")
	flag.StringVar(&options.memprofile, "memprofile", "", "write mem profile to `file`")

	flag.Parse()
}

func buildPluginLink(config map[string]interface{}) []*input.InputBox {
	boxes := make([]*input.InputBox, 0)

	var inputPlugin input.Input
	for input_idx, inputI := range config["inputs"].([]interface{}) {
		outputs := output.BuildOutputs(config)
		filters := filter.BuildFilters(config, nil, outputs)

		i := inputI.(map[interface{}]interface{})
		glog.Infof("input[%d] %v", input_idx+1, i)

		// len(i) is 1
		for inputTypeI, inputConfigI := range i {
			inputType := inputTypeI.(string)
			inputConfig := inputConfigI.(map[interface{}]interface{})

			if len(filters) > 0 {
				inputPlugin = input.GetInput(inputType, inputConfig, filters[0], nil)
			} else {
				inputPlugin = input.GetInput(inputType, inputConfig, nil, outputs)
			}

			box := input.NewInputBox(inputPlugin, outputs)
			boxes = append(boxes, box)
		}
	}
	return boxes
}

func main() {
	if options.pprof {
		go func() {
			http.ListenAndServe(options.pprofAddr, nil)
		}()
	}
	if options.cpuprofile != "" {
		f, err := os.Create(options.cpuprofile)
		if err != nil {
			glog.Fatalf("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			glog.Fatalf("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	if options.memprofile != "" {
		f, err := os.Create(options.memprofile)
		if err != nil {
			glog.Fatalf("could not create memory profile: ", err)
		}
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			glog.Fatalf("could not write memory profile: ", err)
		}
		f.Close()
	}

	config, err := parseConfig(options.config)
	if err != nil {
		glog.Fatalf("could not parse config:%s", err)
	}
	glog.Infof("%v", config)

	boxes := buildPluginLink(config)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for {
			<-c
			signal.Stop(c)
			for _, box := range boxes {
				glog.Info(box)
				box.Shutdown()
			}
			os.Exit(0)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(len(boxes))
	defer wg.Wait()

	for _, box := range boxes {
		go func() {
			defer wg.Done()
			glog.Info(box)
			box.Beat()
		}()
	}
}
