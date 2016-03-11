package metrics

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cyberdelia/go-metrics-graphite"
	"github.com/op/go-logging"
	"github.com/rcrowley/go-metrics"
)

var (
	log *logging.Logger

	activeESRequests   = metrics.NewRegisteredGauge("es.requests.active", metrics.DefaultRegistry)
	failedESRequests   = metrics.NewRegisteredMeter("es.requests.failed", metrics.DefaultRegistry)
	failedESTraces = metrics.NewRegisteredMeter("es.traces.failed", metrics.DefaultRegistry)
	activeHTTPRequests = metrics.NewRegisteredGauge("http.requests.active", metrics.DefaultRegistry)
	spansReceived      = metrics.NewRegisteredMeter("http.spans.received", metrics.DefaultRegistry)
	tracesPending      = metrics.NewRegisteredGauge("http.traces.pending", metrics.DefaultRegistry)
	tracesCompleted    = metrics.NewRegisteredMeter("http.traces.completed", metrics.DefaultRegistry)
	tracesUncompleted  = metrics.NewRegisteredMeter("http.traces.uncompleted", metrics.DefaultRegistry)

	// ActiveESRequests store active ES requests counter
	ActiveESRequests int64

	// ActiveHTTPRequests store active HTTP requests counter
	ActiveHTTPRequests int64

	// FailedESRequests store failed ES requests counter
	FailedESRequests int64
	
	// FailedESTraces store failed ES trace documents
	FailedESTraces int64

	// FlushTimer meter collect pending traces time
	FlushTimer = metrics.NewRegisteredTimer("flush.time", metrics.DefaultRegistry)

	// SpansReceived store received spans counter
	SpansReceived int64

	// TracesCompleted store completed traces counter
	TracesCompleted int64

	// TracesUncompleted store uncompleted traces counter
	TracesUncompleted int64

	// TracesPending store pending traces counter
	TracesPending int64
)

// Config is settings for graphite
type Config struct {
	Enable         bool   `yaml:"enable"`
	GraphiteHost   string `yaml:"graphite_host"`
	GraphitePort   int    `yaml:"graphite_port"`
	GraphitePrefix string `yaml:"graphite_prefix"`
}

//SetLogger allows to set up external logger
func SetLogger(logger *logging.Logger) {
	log = logger
}

// InitStats initialize sending statistics to graphite
func InitStats(conf *Config) error {

	if !conf.Enable {
		log.Info("Graphite statistics is disabled")
		return nil
	}

	if conf.GraphiteHost == "" {
		log.Warning("Graphite host is not configured")
		return nil
	}

	graphiteAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", conf.GraphiteHost, conf.GraphitePort))
	if err != nil {
		log.Errorf("Can not resolve graphite_host: %s [%s]", conf.GraphiteHost, err)
		return err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	shortname := strings.Split(hostname, ".")[0]

	go func() {
		for {
			activeESRequests.Update(atomic.LoadInt64(&ActiveESRequests))
			activeHTTPRequests.Update(atomic.LoadInt64(&ActiveHTTPRequests))
			failedESTraces.Mark(atomic.SwapInt64(&FailedESTraces, 0))
			failedESRequests.Mark(atomic.SwapInt64(&FailedESRequests, 0))
			spansReceived.Mark(atomic.SwapInt64(&SpansReceived, 0))
			tracesPending.Update(atomic.SwapInt64(&TracesPending, 0))
			tracesCompleted.Mark(atomic.SwapInt64(&TracesCompleted, 0))
			tracesUncompleted.Mark(atomic.SwapInt64(&TracesUncompleted, 0))
			time.Sleep(10 * time.Second)
		}
	}()

	go graphite.Graphite(metrics.DefaultRegistry, 60*time.Second, fmt.Sprintf("%s.go-dtrace.%s", conf.GraphitePrefix, shortname), graphiteAddr)
	return nil
}
