// Copyright 2014 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus_test

import (
	"flag"
	"fmt"
	"math"
	"net/http"
	"runtime"
	"sort"

	dto "github.com/prometheus/client_model/go"

	"code.google.com/p/goprotobuf/proto"

	"github.com/prometheus/client_golang/prometheus"
)

func ExampleGauge() {
	opsQueued := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "our_company",
		Subsystem: "blob_storage",
		Name:      "ops_queued",
		Help:      "Number of blob storage operations waiting to be processed.",
	})
	prometheus.MustRegister(opsQueued)

	// 10 operations queued by the goroutine managing incoming requests.
	opsQueued.Add(10)
	// A worker goroutine has picked up a waiting operation.
	opsQueued.Dec()
	// And once more...
	opsQueued.Dec()
}

func ExampleGaugeVec() {
	binaryVersion := flag.String("binary_version", "debug", "Version of the binary: debug, canary, production.")
	flag.Parse()

	opsQueued := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   "our_company",
			Subsystem:   "blob_storage",
			Name:        "ops_queued",
			Help:        "Number of blob storage operations waiting to be processed, partitioned by user and type.",
			ConstLabels: prometheus.Labels{"binary_version": *binaryVersion},
		},
		[]string{
			// Which user has requested the operation?
			"user",
			// Of what type is the operation?
			"type",
		},
	)
	prometheus.MustRegister(opsQueued)

	// Increase a value using compact (but order-sensitive!) WithLabelValues().
	opsQueued.WithLabelValues("bob", "put").Add(4)
	// Increase a value with a map using WithLabels. More verbose, but order
	// doesn't matter anymore.
	opsQueued.With(prometheus.Labels{"type": "delete", "user": "alice"}).Inc()
}

func ExampleGaugeFunc() {
	if err := prometheus.Register(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Subsystem: "runtime",
			Name:      "goroutines_count",
			Help:      "Number of goroutines that currently exist.",
		},
		func() float64 { return float64(runtime.NumGoroutine()) },
	)); err == nil {
		fmt.Println("GaugeFunc 'goroutines_count' registered.")
	}
	// Note that the count of goroutines is a gauge (and not a counter) as
	// it can go up and down.

	// Output:
	// GaugeFunc 'goroutines_count' registered.
}

func ExampleCounter() {
	pushCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "repository_pushes", // Note: No help string...
	})
	err := prometheus.Register(pushCounter) // ... so this will return an error.
	if err != nil {
		fmt.Println("Push counter couldn't be registered, no counting will happen:", err)
		return
	}

	// Try it once more, this time with a help string.
	pushCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "repository_pushes",
		Help: "Number of pushes to external repository.",
	})
	err = prometheus.Register(pushCounter)
	if err != nil {
		fmt.Println("Push counter couldn't be registered AGAIN, no counting will happen:", err)
		return
	}

	pushComplete := make(chan struct{})
	// TODO: Start a goroutine that performs repository pushes and reports
	// each completion via the channel.
	for _ = range pushComplete {
		pushCounter.Inc()
	}
	// Output:
	// Push counter couldn't be registered, no counting will happen: descriptor Desc{fqName: "repository_pushes", help: "", constLabels: {}, variableLabels: []} is invalid: empty help string
}

func ExampleCounterVec() {
	binaryVersion := flag.String("environment", "test", "Execution environment: test, staging, production.")
	flag.Parse()

	httpReqs := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "http_requests_total",
			Help:        "How many HTTP requests processed, partitioned by status code and http method.",
			ConstLabels: prometheus.Labels{"env": *binaryVersion},
		},
		[]string{"code", "method"},
	)
	prometheus.MustRegister(httpReqs)

	httpReqs.WithLabelValues("404", "POST").Add(42)

	// If you have to access the same set of labels very frequently, it
	// might be good to retrieve the metric only once and keep a handle to
	// it. But beware of deletion of that metric, see below!
	m := httpReqs.WithLabelValues("200", "GET")
	for i := 0; i < 1000000; i++ {
		m.Inc()
	}
	// Delete a metric from the vector. If you have previously kept a handle
	// to that metric (as above), future updates via that handle will go
	// unseen (even if you re-create a metric with the same label set
	// later).
	httpReqs.DeleteLabelValues("200", "GET")
	// Same thing with the more verbose Labels syntax.
	httpReqs.Delete(prometheus.Labels{"method": "GET", "code": "200"})
}

func ExampleInstrumentHandler() {
	// Handle the "/doc" endpoint with the standard http.FileServer handler.
	// By wrapping the handler with InstrumentHandler, request count,
	// request and response sizes, and request latency are automatically
	// exported to Prometheus, partitioned by HTTP status code and method
	// and by the handler name (here "fileserver").
	http.Handle("/doc", prometheus.InstrumentHandler(
		"fileserver", http.FileServer(http.Dir("/usr/share/doc")),
	))
	// The Prometheus handler still has to be registered to handle the
	// "/metrics" endpoint. The handler returned by prometheus.Handler() is
	// already instrumented - with "prometheus" as the handler name. In this
	// example, we want the handler name to be "metrics", so we instrument
	// the uninstrumented Prometheus handler ourselves.
	http.Handle("/metrics", prometheus.InstrumentHandler(
		"metrics", prometheus.UninstrumentedHandler(),
	))
}

func ExampleLabelPairSorter() {
	labelPairs := []*dto.LabelPair{
		&dto.LabelPair{Name: proto.String("status"), Value: proto.String("404")},
		&dto.LabelPair{Name: proto.String("method"), Value: proto.String("get")},
	}

	sort.Sort(prometheus.LabelPairSorter(labelPairs))

	fmt.Println(labelPairs)
	// Output:
	// [name:"method" value:"get"  name:"status" value:"404" ]
}

func ExampleRegister() {
	// Imagine you have a worker pool and want to count the tasks completed.
	taskCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: "worker_pool",
		Name:      "completed_tasks_total",
		Help:      "Total number of tasks completed.",
	})
	// This will register fine.
	if err := prometheus.Register(taskCounter); err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("taskCounter registered.")
	}
	// Don't forget to tell the HTTP server about the Prometheus handler.
	// (In a real program, you still need to start the http server...)
	http.Handle("/metrics", prometheus.Handler())

	// Now you can start workers and give every one of them a pointer to
	// taskCounter and let it increment it whenever it completes a task.
	taskCounter.Inc() // This has to happen somewhere in the worker code.

	// But wait, you want to see how individual workers perform. So you need
	// a vector of counters, with one element for each worker.
	taskCounterVec := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "worker_pool",
			Name:      "completed_tasks_total",
			Help:      "Total number of tasks completed.",
		},
		[]string{"worker_id"},
	)

	// Registering will fail because we already have a metric of that name.
	if err := prometheus.Register(taskCounterVec); err != nil {
		fmt.Println("taskCounterVec not registered:", err)
	} else {
		fmt.Println("taskCounterVec registered.")
	}

	// To fix, first unregister the old taskCounter.
	if prometheus.Unregister(taskCounter) {
		fmt.Println("taskCounter unregistered.")
	}

	// Try registering taskCounterVec again.
	if err := prometheus.Register(taskCounterVec); err != nil {
		fmt.Println("taskCounterVec not registered:", err)
	} else {
		fmt.Println("taskCounterVec registered.")
	}
	// Bummer! Still doesn't work.

	// Prometheus will not allow you to ever export metrics with
	// inconsistent help strings or label names. After unregistering, the
	// unregistered metrics will cease to show up in the /metrics http
	// response, but the registry still remembers that those metrics had
	// been exported before. For this example, we will now choose a
	// different name. (In a real program, you would obviously not export
	// the obsolete metric in the first place.)
	taskCounterVec = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: "worker_pool",
			Name:      "completed_tasks_by_id",
			Help:      "Total number of tasks completed.",
		},
		[]string{"worker_id"},
	)
	if err := prometheus.Register(taskCounterVec); err != nil {
		fmt.Println("taskCounterVec not registered:", err)
	} else {
		fmt.Println("taskCounterVec registered.")
	}
	// Finally it worked!

	// The workers have to tell taskCounterVec their id to increment the
	// right element in the metric vector.
	taskCounterVec.WithLabelValues("42").Inc() // Code from worker 42.

	// Each worker could also keep a reference to their own counter element
	// around. Pick the counter at initialization time of the worker.
	myCounter := taskCounterVec.WithLabelValues("42") // From worker 42 initialization code.
	myCounter.Inc()                                   // Somewhere in the code of that worker.

	// Note that something like WithLabelValues("42", "spurious arg") would
	// panic (because you have provided too many label values). If you want
	// to get an error instead, use GetMetricWithLabelValues(...) instead.
	notMyCounter, err := taskCounterVec.GetMetricWithLabelValues("42", "spurious arg")
	if err != nil {
		fmt.Println("Worker initialization failed:", err)
	}
	if notMyCounter == nil {
		fmt.Println("notMyCounter is nil.")
	}

	// A different (and somewhat tricky) approach is to use
	// ConstLabels. ConstLabels are pairs of label names and label values
	// that never change. You might ask what those labels are good for (and
	// rightfully so - if they never change, they could as well be part of
	// the metric name). There are essentially two use-cases: The first is
	// if labels are constant throughout the lifetime of a binary execution,
	// but they vary over time or between different instances of a running
	// binary. The second is what we have here: Each worker creates and
	// registers an own Counter instance where the only difference is in the
	// value of the ConstLabels. Those Counters can all be registered
	// because the different ConstLabel values guarantee that each worker
	// will increment a different Counter metric.
	counterOpts := prometheus.CounterOpts{
		Subsystem:   "worker_pool",
		Name:        "completed_tasks",
		Help:        "Total number of tasks completed.",
		ConstLabels: prometheus.Labels{"worker_id": "42"},
	}
	taskCounterForWorker42 := prometheus.NewCounter(counterOpts)
	if err := prometheus.Register(taskCounterForWorker42); err != nil {
		fmt.Println("taskCounterVForWorker42 not registered:", err)
	} else {
		fmt.Println("taskCounterForWorker42 registered.")
	}
	// Obviously, in real code, taskCounterForWorker42 would be a member
	// variable of a worker struct, and the "42" would be retrieved with a
	// GetId() method or something. The Counter would be created and
	// registered in the initialization code of the worker.

	// For the creation of the next Counter, we can recycle
	// counterOpts. Just change the ConstLabels.
	counterOpts.ConstLabels = prometheus.Labels{"worker_id": "2001"}
	taskCounterForWorker2001 := prometheus.NewCounter(counterOpts)
	if err := prometheus.Register(taskCounterForWorker2001); err != nil {
		fmt.Println("taskCounterVForWorker2001 not registered:", err)
	} else {
		fmt.Println("taskCounterForWorker2001 registered.")
	}

	taskCounterForWorker2001.Inc()
	taskCounterForWorker42.Inc()
	taskCounterForWorker2001.Inc()

	// Yet another approach would be to turn the workers themselves into
	// Collectors and register them. See the Collector example for details.

	// Output:
	// taskCounter registered.
	// taskCounterVec not registered: a previously registered descriptor with the same fully-qualified name as Desc{fqName: "worker_pool_completed_tasks_total", help: "Total number of tasks completed.", constLabels: {}, variableLabels: [worker_id]} has different label names or a different help string
	// taskCounter unregistered.
	// taskCounterVec not registered: a previously registered descriptor with the same fully-qualified name as Desc{fqName: "worker_pool_completed_tasks_total", help: "Total number of tasks completed.", constLabels: {}, variableLabels: [worker_id]} has different label names or a different help string
	// taskCounterVec registered.
	// Worker initialization failed: inconsistent label cardinality
	// notMyCounter is nil.
	// taskCounterForWorker42 registered.
	// taskCounterForWorker2001 registered.
}

func ExampleSummary() {
	temps := prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "pond_temperature_celsius",
		Help: "The temperature of the frog pond.", // Sorry, we can't measure how badly it smells.
	})

	// Simulate some observations.
	for i := 0; i < 1000; i++ {
		temps.Observe(30 + math.Floor(120*math.Sin(float64(i)*0.1))/10)
	}

	// Just for demonstration, let's check the state of the summary by
	// (ab)using its Write method (which is usually only used by Prometheus
	// internally).
	metric := &dto.Metric{}
	temps.Write(metric)
	fmt.Println(proto.MarshalTextString(metric))

	// Output:
	// summary: <
	//   sample_count: 1000
	//   sample_sum: 29969.50000000001
	//   quantile: <
	//     quantile: 0.5
	//     value: 31.1
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 41.3
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 41.9
	//   >
	// >
}

func ExampleSummaryVec() {
	temps := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "pond_temperature_celsius",
			Help: "The temperature of the frog pond.", // Sorry, we can't measure how badly it smells.
		},
		[]string{"species"},
	)

	// Simulate some observations.
	for i := 0; i < 1000; i++ {
		temps.WithLabelValues("litoria-caerulea").Observe(30 + math.Floor(120*math.Sin(float64(i)*0.1))/10)
		temps.WithLabelValues("lithobates-catesbeianus").Observe(32 + math.Floor(100*math.Cos(float64(i)*0.11))/10)
	}

	// Just for demonstration, let's check the state of the summary vector
	// by (ab)using its Collect method and the Write method of its elements
	// (which is usually only used by Prometheus internally - code like the
	// following will never appear in your own code).
	metricChan := make(chan prometheus.Metric)
	go func() {
		defer close(metricChan)
		temps.Collect(metricChan)
	}()

	metricStrings := []string{}
	for metric := range metricChan {
		dtoMetric := &dto.Metric{}
		metric.Write(dtoMetric)
		metricStrings = append(metricStrings, proto.MarshalTextString(dtoMetric))
	}
	sort.Strings(metricStrings) // For reproducible print order.
	fmt.Println(metricStrings)

	// Output:
	// [label: <
	//   name: "species"
	//   value: "lithobates-catesbeianus"
	// >
	// summary: <
	//   sample_count: 1000
	//   sample_sum: 31956.100000000017
	//   quantile: <
	//     quantile: 0.5
	//     value: 32.4
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 41.4
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 41.9
	//   >
	// >
	//  label: <
	//   name: "species"
	//   value: "litoria-caerulea"
	// >
	// summary: <
	//   sample_count: 1000
	//   sample_sum: 29969.50000000001
	//   quantile: <
	//     quantile: 0.5
	//     value: 31.1
	//   >
	//   quantile: <
	//     quantile: 0.9
	//     value: 41.3
	//   >
	//   quantile: <
	//     quantile: 0.99
	//     value: 41.9
	//   >
	// >
	// ]
}
