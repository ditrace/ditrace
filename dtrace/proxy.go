package dtrace

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const xProxyHeader string = "X-DTrace-Proxy"

// HTTPClient is client to send proxy requests
type HTTPClient interface {
	Send(*http.Request) error
}

type realHTTPClient struct {
	client *http.Client
}

// ProxyClient implementing HTTPClient interface
var ProxyClient HTTPClient = &realHTTPClient{
	client: &http.Client{
		Timeout: time.Second * 5,
	},
}

func (proxyClient *realHTTPClient) Send(req *http.Request) error {
	res, err := proxyClient.client.Do(req)
	if err == nil && res.StatusCode != 200 {
		body, _ := ioutil.ReadAll(res.Body)
		return fmt.Errorf("response code %d [%s]", res.StatusCode, string(body))
	}
	return err
}

// Proxy server distributes traces among all replicas
func Proxy(proxyChannel, toLoadBalancer chan *Span, config *Config) {
	var wg sync.WaitGroup
	defer wg.Wait()

	channels := make([]chan *Span, len(config.Replicas))

	for i := range channels {
		if i == config.MyReplicaIndex {
			channels[i] = toLoadBalancer
		} else {
			ch := make(chan *Span)
			channels[i] = ch
			wg.Add(1)
			go func(j int) {
				defer wg.Done()
				replicaSender(ch, config.Replicas[j], 100, time.Second)
			}(i)
		}
	}

	for span := range proxyChannel {
		if span.TraceHashSum%config.Sampling != 0 {
			continue
		}
		replicaIndex := span.TraceHashSum % uint64(len(config.Replicas))
		channels[replicaIndex] <- span
	}

	for _, ch := range channels {
		close(ch)
	}
}

func replicaSender(ch chan *Span, replica string, bulkSize int, interval time.Duration) {
	replicaURL, err := url.Parse(replica)
	if err != nil {
		log.Panicf("Invalid replica url [%s]: %s", replica, err)
	}
	replicaURL.Path = "/spans"

	log.Infof("Started replica sender with url %s", replica)

	bulks := make(map[string][]*Span)
	timer := time.NewTimer(interval)
	var (
		wg   sync.WaitGroup
		ok   = true
		span *Span
	)
	defer wg.Wait()
For:
	for {
		select {
		case span, ok = <-ch:
			if !ok {
				break
			}
			bulk, exists := bulks[span.System]
			if !exists {
				bulk = make([]*Span, 0, bulkSize)
			}
			bulk = append(bulk, span)
			bulks[span.System] = bulk
			if len(bulk) >= bulkSize {
				break
			}
			continue For
		case <-timer.C:
			timer = time.NewTimer(interval)
		}

		for system := range bulks {
			wg.Add(1)
			go func(sys string, bulk []*Span) {
				defer wg.Done()
				sendToReplica(sys, bulk, *replicaURL)
			}(system, bulks[system])
		}
		if !ok {
			return
		}
		bulks = make(map[string][]*Span)
	}
}

func sendToReplica(system string, bulk []*Span, replicaURL url.URL) {
	if len(bulk) == 0 {
		return
	}

	var buffer bytes.Buffer
	for _, span := range bulk {
		buffer.Write([]byte(span.RawString))
		buffer.WriteRune('\n')
	}

	v := url.Values{}
	v.Add("system", system)
	replicaURL.RawQuery = v.Encode()

	req := &http.Request{
		Method:     "POST",
		URL:        &replicaURL,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       ioutil.NopCloser(bytes.NewReader(buffer.Bytes())),
		Host:       replicaURL.Host,
	}

	req.ContentLength = int64(buffer.Len())
	req.Header.Add(xProxyHeader, "True")

	if err := ProxyClient.Send(req); err != nil {
		log.Warningf("Can not proxy spans to %s: %s", replicaURL.String(), err)
	}
}
