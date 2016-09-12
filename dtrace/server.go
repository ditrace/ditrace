package dtrace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ditrace/ditrace/metrics"

	"github.com/op/go-logging"
)

var (
	// IgnoreFields is array to ignore incoming unnecessary message fields
	log *logging.Logger
)

//SetLogger allows to set up external logger
func SetLogger(logger *logging.Logger) {
	log = logger
}

// Listen start listening address
func Listen(terminate chan os.Signal, config *Config, started chan bool) {
	if config.Enable {

		log.Infof("Start listening http address [%s] ...", config.Address)

		toLoadBalancer := make(chan *Span)
		toProxy := make(chan *Span)
		toES := make(chan *Document)

		var wg sync.WaitGroup
		defer wg.Wait()
		wg.Add(3)
		go func() {
			defer wg.Done()
			Proxy(toProxy, toLoadBalancer, config)
		}()
		go func() {
			defer wg.Done()
			loadBalancer(toLoadBalancer, toES, config)
		}()
		go func() {
			defer wg.Done()
			elasticSender(toES, config.ElasticSearch, config.BulkSize, time.Second*5)
		}()

		http.HandleFunc("/spans", func(w http.ResponseWriter, r *http.Request) {
			activeRequests := atomic.LoadInt64(&metrics.ActiveESRequests)

			if activeRequests > config.MaxActiveRequests {
				log.Warningf("Handle request failed [%s %s]: too many active requests (%d).", r.Method, r.RequestURI, activeRequests)
				w.WriteHeader(503)
				return
			}

			start := time.Now()

			code, count, err := HandleRequest(config, r, toProxy, toLoadBalancer)

			atomic.AddInt64(&metrics.SpansReceived, int64(count))

			elapsed := time.Since(start)
			remoteAddress := r.RemoteAddr
			realIP := r.Header.Get("X-Real-Ip")
			if len(realIP) > 0 {
				remoteAddress = realIP
			}

			w.Header().Set("Content-Type", "text/plain; charset=utf-8")

			if err != nil {
				log.Warningf("Handle request failed [%s %s %s %d %f]: %s.", r.Method, r.RequestURI, remoteAddress, code, elapsed.Seconds(), err)
				w.WriteHeader(code)
				if _, err := fmt.Fprintln(w, err); err != nil {
					log.Errorf("Failed to sending response: %s", err)
				}
				return
			}

			w.Write([]byte(fmt.Sprintf("%d", count)))

			log.Debugf("%s\t%s\t%s\t%d\t%d\t%.3f", r.Method, r.RequestURI, remoteAddress, count, code, elapsed.Seconds())
		})

		ln, err := net.Listen("tcp", config.Address)
		if err != nil {
			log.Error(err.Error())
			return
		}
		srv := &http.Server{Addr: config.Address}
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv.Serve(ln)
		}()
		started <- true
		<-terminate
		log.Info("Closing http listener")
		ln.Close()

		for {
			time.Sleep(20 * time.Millisecond)
			if atomic.LoadInt64(&metrics.ActiveHTTPRequests) > 0 {
				continue
			}
			break
		}

		close(toProxy)

	} else {

		log.Info("Http server is disabled")
		<-terminate

	}
}

// HandleRequest parse json body and send it to elasticsearch channel
func HandleRequest(config *Config, r *http.Request, toProxy, toLoadBalancer chan *Span) (int, int, error) {
	defer atomic.AddInt64(&metrics.ActiveHTTPRequests, -1)
	atomic.AddInt64(&metrics.ActiveHTTPRequests, 1)

	bodyReader := bufio.NewReader(r.Body)

	var (
		line  bytes.Buffer
		count int
	)

	for {
		part, prefix, err := bodyReader.ReadLine()
		if err == nil {
			line.Write(part)
			if !prefix {

				count++

				var span Span

				lineBytes := line.Bytes()
				line.Reset()
				if count == 1 {
					lineBytes = bytes.Trim(lineBytes, "\xef\xbb\xbf")
				}
				if len(lineBytes) == 0 {
					continue
				}
				if err := json.Unmarshal(lineBytes, &span); err != nil {
					return 400, count, fmt.Errorf("Can not decode line [%s]: %s", string(lineBytes), err)
				}

				span.RawString = string(lineBytes)

				query := r.URL.Query()
				if system := query.Get("system"); len(system) > 0 {
					span.System = system
				}

				if len(span.System) == 0 {
					return 400, count, fmt.Errorf("Field system is required")
				}
				if len(span.TraceID) == 0 {
					return 400, count, fmt.Errorf("Field traceid is required")
				}
				if len(span.ID) == 0 {
					return 400, count, fmt.Errorf("Field spanid is required")
				}

				span.CalculateTraceHashSum()

				if len(r.Header.Get(xProxyHeader)) == 0 {
					toProxy <- &span
				} else {
					toLoadBalancer <- &span
				}
			}
			continue
		}
		if err != io.EOF {
			return 500, count, fmt.Errorf("Can not read body: %s", err)
		}
		break
	}
	return 200, count, nil
}

func loadBalancer(spansChannel chan *Span, toES chan *Document, config *Config) {
	var wg sync.WaitGroup
	channels := make([]chan *Span, runtime.NumCPU()*4)

	for i := range channels {
		traceMap := make(TraceMap)
		ch := make(chan *Span)
		channels[i] = ch
		wg.Add(1)
		go func(ch chan *Span) {
			defer wg.Done()
			timer := time.NewTimer(time.Second * 10)
			for {
				select {
				case span, ok := <-ch:
					if !ok {
						traceMap = traceMap.Collect(0, 0, toES)
						return
					}
					traceMap.Put(span)
				case <-timer.C:
					timer = time.NewTimer(time.Second * 10)
					traceMap.Collect(time.Second*time.Duration(config.MinTTL), time.Second*time.Duration(config.MaxTTL), toES)
				}
			}
		}(ch)
	}

	count := uint64(len(channels))

	log.Infof("Run %d span channels", count)

	for span := range spansChannel {
		i := span.TraceHashSum % count
		ch := channels[i]
		ch <- span
	}

	for _, ch := range channels {
		close(ch)
	}

	wg.Wait()

	close(toES)
}
