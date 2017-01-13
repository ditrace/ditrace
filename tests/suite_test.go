package tests

import (
	"bufio"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ditrace/ditrace/dtrace"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/op/go-logging"
)

func TestDTrace(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DTrace Suite")
}

var _ = Describe("DTrace", func() {
	var (
		log *logging.Logger
	)
	BeforeSuite(func() {
		log, _ = logging.GetLogger("tests")
		dtrace.SetLogger(log)
	})

	Context("Normalize url", func() {
		Context("without hex", func() {
			It("should be normalized without trailing slash", func() {
				Expect("/url").To(Equal(dtrace.NormalizeURL("/url")))
				Expect("/url").To(Equal(dtrace.NormalizeURL("/url/")))
			})
		})
		Context("with hex", func() {
			It("should be replaced correctly", func() {
				Expect("/url/<hex>").To(Equal(dtrace.NormalizeURL("/url/22345200-abe8-4f60-90c8-0d43c5f6c0f6")))
			})
		})
		Context("with number", func() {
			It("should be replaced correctly", func() {
				Expect("/url/<num>").To(Equal(dtrace.NormalizeURL("/url/2234")))
			})
		})
		Context("with hex and number", func() {
			It("should be replaced correctly", func() {
				Expect("/url/<hex>/<num>").To(Equal(dtrace.NormalizeURL("/url/22345200-abe8-4f60-90c8-0d43c5f6c0f6/2234")))
			})
		})
	})

	Context("Put spans", func() {
		var (
			traceMap dtrace.TraceMap
			span     *dtrace.Span
		)
		BeforeEach(func() {
			traceMap = make(dtrace.TraceMap)
		})

		Context("one root span", func() {
			BeforeEach(func() {
				span = &dtrace.Span{
					System:    "system",
					ID:        "spanid",
					TraceID:   "traceid",
					ProfileID: "profileid",
					Annotations: map[string]interface{}{
						"url": "/url/22345200-abe8-4f60-90c8-0d43c5f6c0f6/",
					},
					Timeline: map[string]dtrace.SpanTimestamp{
						"ss": dtrace.SpanTimestamp{Value: mustParseTime("2015-07-24T09:18:26.8917899Z")},
						"sr": dtrace.SpanTimestamp{Value: mustParseTime("2015-07-24T09:18:26.3023890Z")},
					},
				}
			})

			It("should be aggregated correctly", func() {
				trace := traceMap.Put(span)
				Expect(trace.Root.Duration()).To(Equal(int64(589400)))
				Expect(trace.Completed).To(BeTrue())
				Expect(trace.Spans["spanid"].Annotations["url"]).To(Equal("/url/<hex>"))
				Expect(trace.Spans["spanid"].Annotations["rawurl"]).To(Equal("/url/22345200-abe8-4f60-90c8-0d43c5f6c0f6/"))
				documents := trace.GetESDocuments()
				Expect(documents).To(HaveLen(1))
				Expect(documents[0].Index).To(Equal("traces-2015.07.24"))
				Expect(documents[0].Fields["system"]).To(Equal("system"))
				Expect(documents[0].Fields["duration"]).To(Equal(int64(589400)))
				Expect(documents[0].Fields["profileid"]).To(Equal("profileid"))
				Expect(documents[0].Fields["spans"].([]map[string]interface{})).ToNot(BeNil())
				spans := documents[0].Fields["spans"].([]map[string]interface{})
				Expect(spans[0]["timeline"].(map[string]string)["ss"]).To(Equal("2015-07-24T09:18:26.8917899Z"))
			})

			It("should not be collected too early", func() {
				traceMap.Put(span)
				traceMap = traceMap.Collect(time.Second*10, time.Second*10, 1000, make(chan *dtrace.Document))
				Expect(traceMap).To(HaveLen(1))
			})
		})

		Context("Annotations overwriting", func() {
			var (
				jsons []string
				trace *dtrace.Trace
			)

			It("should be for new span revision", func() {
				jsons = []string{
					`{"traceid": "0", "spanid": "spanid", "annotations": {"targetid": "wrong"}}`,
					`{"traceid": "0", "spanid": "spanid", "annotations": {"targetid": "correct", "revision": 1}}`,
				}
			})

			It("should be for client span", func() {
				jsons = []string{
					`{"traceid": "0", "spanid": "spanid", "annotations": {"targetid": "wrong"} , "timeline":{"sr":"2016-03-03T05:38:05.9538750Z"}}`,
					`{"traceid": "0", "spanid": "spanid", "annotations": {"targetid": "correct"}, "timeline":{"cs":"2016-03-03T05:38:05.9538750Z"}}`,
				}
			})

			It("should be for server span", func() {
				jsons = []string{
					`{"traceid": "0", "spanid": "spanid", "annotations": {"targetid": "correct"} , "timeline":{"cs":"2016-03-03T05:38:05.9538750Z"}}`,
					`{"traceid": "0", "spanid": "spanid", "annotations": {"targetid": "wrong"}, "timeline":{"sr":"2016-03-03T05:38:05.9538750Z"}}`,
				}
			})

			AfterEach(func() {
				for _, j := range jsons {
					var span dtrace.Span
					json.Unmarshal([]byte(j), &span)
					trace = traceMap.Put(&span)
				}
				Expect(trace.Spans["spanid"].Annotations["targetid"]).To(Equal("correct"))
			})
		})

		Context("Uncompleted trace", func() {
			var (
				span *dtrace.Span
				toES chan *dtrace.Document
			)
			BeforeEach(func() {
				span = &dtrace.Span{
					System:       "system",
					TraceID:      "traceid",
					ID:           "spanid",
					ParentSpanID: "parentspanid",
				}
				toES = make(chan *dtrace.Document)
			})

			It("should be wasted on timeout", func() {
				traceMap.Put(span)
				traceMap = traceMap.Collect(0, 0, 1000, toES)
				Expect(traceMap).To(HaveLen(0))
			})

			It("should be waited for complete", func() {
				traceMap.Put(span)
				traceMap = traceMap.Collect(0, time.Minute, 1000, toES)
				Expect(traceMap).To(HaveLen(1))
			})

			It("should cleanup overflow trace", func() {
				traceMap.Put(span)
				traceMap = traceMap.Collect(0, time.Minute, 0, toES)
				Expect(traceMap).To(HaveLen(0))
			})
		})

		Context("spans from spans.txt", func() {
			var trace *dtrace.Trace
			BeforeEach(func() {
				trace = fileFeed("spans.txt", traceMap)
			})
			It("should be converted to ES documents correctly", func() {
				Expect(traceMap["f03d7cf3b36145d0a2591d11d2e93025"].Spans["9cb62bfb"].Annotations["targetid"]).To(Equal("GenerateBoxesTaskData"))
				documents := trace.GetESDocuments()
				spans := documents[0].Fields["spans"].([]map[string]interface{})
				chains := documents[0].Fields["chains"].([]*dtrace.Chain)
				Expect(documents).To(HaveLen(1))
				Expect(documents[0].Fields["timestamp"].(time.Time).UnixNano()).To(Equal(int64(1438263809678249600)))
				Expect(spans).To(HaveLen(228))
				Expect(chains).To(HaveLen(225))
			})
		})

		Context("spans from task.txt", func() {
			var trace *dtrace.Trace
			BeforeEach(func() {
				trace = fileFeed("task.txt", traceMap)
			})
			It("should be converted to multiple ES documents", func() {
				Expect(traceMap["8695051faea44d9694fab3c23c2c32e8"].Spans["xxx-xxx-xx-x"].Annotations["targetid"]).To(Equal("task"))
				documents := trace.GetESDocuments()
				Expect(documents).To(HaveLen(2))
			})
		})

		Context("partial span information", func() {
			BeforeEach(func() {
				for _, j := range testPartialJsons {
					var span dtrace.Span
					json.Unmarshal([]byte(j), &span)
					_ = traceMap.Put(&span)
				}
			})
			It("should be aggregated correctly", func() {
				Expect(traceMap["432fa4b867ef41f882f6f575b0b1e180"].Spans["4ee8c1d5"].ParentSpanID).To(Equal("f51f8e99-adbd-4b86-915e-d2175c5b29d6"))
			})
		})

		Context("chain calculation of single root span", func() {
			var trace *dtrace.Trace
			BeforeEach(func() {
				jsons := []string{
					`{"traceid": "0", "spanid": "0", "annotations": {"targetid": "s0", "root": true}}`,
				}
				for _, j := range jsons {
					var span dtrace.Span
					json.Unmarshal([]byte(j), &span)
					trace = traceMap.Put(&span)
				}
			})
			It("should be correct", func() {
				spans, chains, err := trace.GetChains("0")
				Expect(err).ShouldNot(HaveOccurred())

				Expect(spans).To(HaveLen(1))
				Expect(chains).To(HaveLen(1))

				Expect(containsPath(chains, "s0")).To(Equal(1))

				Expect(containsPrefix(chains, "")).To(Equal(1))

				Expect(chains[0].Level).To(Equal(0))
			})
		})

		Context("chain calculation with wrapper", func() {
			var trace *dtrace.Trace
			BeforeEach(func() {
				jsons := []string{
					`{"traceid": "0", "spanid": "0", "annotations": {"targetid": "s0"}}`,
					`{"traceid": "0", "spanid": "1", "parentspanid": "0", "annotations": {"targetid": "s1-w", "wrapper": ""}}`,
					`{"traceid": "0", "spanid": "2", "parentspanid": "1", "annotations": {"targetid": "s1"}}`,
					`{"traceid": "0", "spanid": "3", "parentspanid": "1", "annotations": {"targetid": "s1"}}`,
					`{"traceid": "0", "spanid": "4", "parentspanid": "2", "annotations": {"targetid": "s2"}}`,
					`{"traceid": "0", "spanid": "5", "parentspanid": "3", "annotations": {"targetid": "s2"}}`,
					`{"traceid": "0", "spanid": "6", "parentspanid": "5", "annotations": {"targetid": "s3"}}`,
					`{"traceid": "0", "spanid": "7", "parentspanid": "5", "annotations": {"targetid": "s4"}}`,
				}
				for _, j := range jsons {
					var span dtrace.Span
					json.Unmarshal([]byte(j), &span)
					trace = traceMap.Put(&span)
				}
			})
			It("should be correct", func() {
				spans, chains, err := trace.GetChains("0")
				Expect(err).ShouldNot(HaveOccurred())

				Expect(spans).To(HaveLen(8))
				Expect(chains).To(HaveLen(6))

				Expect(spans[0].ParentSpanID).To(Equal(""))
				Expect(spans[1].ParentSpanID).To(Equal("0"))

				Expect(containsPath(chains, "s0")).To(Equal(1))
				Expect(containsPath(chains, "s0->s1-w")).To(Equal(1))
				Expect(containsPath(chains, "s1->s2")).To(Equal(2))
				Expect(containsPath(chains, "s2->s3")).To(Equal(1))
				Expect(containsPath(chains, "s2->s4")).To(Equal(1))

				Expect(containsPrefix(chains, "")).To(Equal(1))
				Expect(containsPrefix(chains, "s0")).To(Equal(1))
				Expect(containsPrefix(chains, "s0->s0->s1-w")).To(Equal(2))
				Expect(containsPrefix(chains, "s0->s0->s1-w->s1->s2")).To(Equal(2))

				Expect(chains[0].Level).To(Equal(0))
				Expect(chains[1].Level).To(Equal(1))
				Expect(chains[2].Level).To(Equal(2))
				Expect(chains[3].Level).To(Equal(2))
				Expect(chains[4].Level).To(Equal(3))
				Expect(chains[5].Level).To(Equal(3))
			})
		})

		Context("chain calculation targetid patching", func() {
			var trace *dtrace.Trace
			var chains []*dtrace.Chain
			BeforeEach(func() {
				jsons := []string{
					`{"traceid": "0", "spanid": "0", "annotations": {"targetid": "root"}}`,
					`{"traceid": "0", "spanid": "1", "parentspanid": "0", "annotations": {"targetid": "wrapper", "wrapper": ""}}`,
					`{"traceid": "0", "spanid": "2", "parentspanid": "1", "annotations": {}}`,
					`{"traceid": "0", "spanid": "3", "parentspanid": "1", "annotations": {"host": "host"}}`,
					`{"traceid": "0", "spanid": "4", "parentspanid": "1", "annotations": {"host": "host", "targethost": "targethost"}}`,
					`{"traceid": "0", "spanid": "5", "parentspanid": "1", "annotations": {"host": "host", "targethost": "targethost", "targetid": "targetid"}}`,
					`{"traceid": "0", "spanid": "6", "parentspanid": "2", "annotations": {"targetid": "leaf"}}`,
					`{"traceid": "0", "spanid": "7", "parentspanid": "3", "annotations": {"targetid": "leaf"}}`,
					`{"traceid": "0", "spanid": "8", "parentspanid": "4", "annotations": {"targetid": "leaf"}}`,
					`{"traceid": "0", "spanid": "9", "parentspanid": "5", "annotations": {"targetid": "leaf"}}`,
					`{"traceid": "0", "spanid": "10", "parentspanid": "0", "annotations": {"targetid": "nonwrapper"}}`,
					`{"traceid": "0", "spanid": "11", "parentspanid": "10", "annotations": {}}`,
					`{"traceid": "0", "spanid": "12", "parentspanid": "11", "annotations": {"targetid": "leaf"}}`,
				}
				for _, j := range jsons {
					var span dtrace.Span
					json.Unmarshal([]byte(j), &span)
					trace = traceMap.Put(&span)
				}
				_, chains, _ = trace.GetChains("0")
			})

			It("should inherit from wrapper if no local annotations are available", func() {
				Expect(containsPath(chains, "wrapper->leaf")).To(Equal(1))
			})

			It("should not inherit and instead choose appropriate local annotation", func() {
				Expect(containsPath(chains, "host->leaf")).To(Equal(1))
				Expect(containsPath(chains, "targethost->leaf")).To(Equal(1))
				Expect(containsPath(chains, "targetid->leaf")).To(Equal(1))
			})

			It("should not inherit from non-wrapper", func() {
				Expect(containsPath(chains, "root->nonwrapper->unknown->leaf")).To(Equal(1))
			})
		})

		Context("chain calculation without wrapper", func() {
			var trace *dtrace.Trace
			BeforeEach(func() {
				jsons := []string{
					`{"traceid": "0", "spanid": "0", "annotations": {"targetid": "s0"}}`,
					`{"traceid": "0", "spanid": "1", "parentspanid": "0", "annotations": {"targetid": "s1"}}`,
					`{"traceid": "0", "spanid": "2", "parentspanid": "0", "annotations": {"targetid": "s2"}}`,
				}
				for _, j := range jsons {
					var span dtrace.Span
					json.Unmarshal([]byte(j), &span)
					trace = traceMap.Put(&span)
				}
			})
			It("should be correct", func() {
				spans, chains, err := trace.GetChains("0")
				Expect(err).ShouldNot(HaveOccurred())

				Expect(spans).To(HaveLen(3))
				Expect(chains).To(HaveLen(3))

				Expect(containsPath(chains, "s0")).To(Equal(1))
				Expect(containsPath(chains, "s0->s1")).To(Equal(1))
				Expect(containsPath(chains, "s0->s2")).To(Equal(1))

				Expect(containsPrefix(chains, "")).To(Equal(1))
				Expect(containsPrefix(chains, "s0")).To(Equal(2))

				Expect(chains[0].Level).To(Equal(0))
				Expect(chains[1].Level).To(Equal(1))
				Expect(chains[2].Level).To(Equal(1))
			})
		})
	})

	Context("Handle request", func() {
		// TODO: multiline test with BOM
		Context("Handle http requests", func() {
			var (
				toBalancer  chan *dtrace.Span
				code, count int
				err         error
				config      dtrace.Config
				spans       []*dtrace.Span
				wg          sync.WaitGroup
			)
			BeforeEach(func() {
				toBalancer = make(chan *dtrace.Span)
				spans = make([]*dtrace.Span, 0)
				wg.Add(1)
				go func(ch chan *dtrace.Span) {
					defer wg.Done()
					for {
						span, ok := <-ch
						if !ok {
							break
						}
						spans = append(spans, span)
					}
				}(toBalancer)
			})
			Context("Request is valid", func() {
				BeforeEach(func() {
					code, count, err = dtrace.HandleRequest(&config, makeRequest(`{
							"annotations": { "url": "generateboxestaskdata" },
							"timeline": {
								"ss": "2015-07-24T09:18:26.8917899Z",
								"sr": "2015-07-24T09:18:26.3023890Z"
							},
							"targetid": "task",
							"spanid": "549f7d28",
							"traceid": "a7b061a83160443b994022d707d1443d"
						}`), toBalancer, toBalancer)
					close(toBalancer)
					wg.Wait()
				})
				It("should return 200", func() {
					Expect(err).ShouldNot(HaveOccurred())
					Expect(code).To(Equal(200))
					Expect(count).To(Equal(1))
				})
				It("should parse span correctly", func() {
					Expect(spans).To(HaveLen(1))
					Expect(spans[0].IsRoot()).To(Equal(false))
					Expect(spans[0].ParentSpanID).To(Equal(""))
					Expect(spans[0].ID).To(Equal("549f7d28"))
					Expect(spans[0].TraceID).To(Equal("a7b061a83160443b994022d707d1443d"))
					Expect(spans[0].Annotations["url"]).To(Equal("generateboxestaskdata"))
					Expect(spans[0].Timeline["ss"].Value.UnixNano()).To(Equal(int64(1437729506891789900)))
					Expect(spans[0].Timeline["sr"].Value.UnixNano()).To(Equal(int64(1437729506302389000)))
				})
			})

			Context("Request is valid and contains BOM", func() {
				BeforeEach(func() {
					code, count, err = dtrace.HandleRequest(&config, makeRequest("\xef\xbb\xbf"+`{
							"annotations": { "url": "generateboxestaskdata" },
							"timeline": {
								"ss": "2015-07-24T09:18:26.8917899Z",
								"sr": "2015-07-24T09:18:26.3023890Z"
							},
							"targetid": "task",
							"spanid": "549f7d28",
							"traceid": "a7b061a83160443b994022d707d1443d"
						}`), toBalancer, toBalancer)
					close(toBalancer)
					wg.Wait()
				})
				It("should return 200", func() {
					Expect(err).ShouldNot(HaveOccurred())
					Expect(code).To(Equal(200))
					Expect(count).To(Equal(1))
				})
			})
		})
	})

	Context("Proxy", func() {
		var (
			toProxy, toLoadBalancer chan *dtrace.Span
			config                  = &dtrace.Config{
				Replicas:       []string{"http://replica1:5678", "http://replica2:1234", "https://replica3"},
				MyReplicaIndex: 1,
				Sampling:       1,
			}
			span       *dtrace.Span
			spans      []*dtrace.Span
			wg         sync.WaitGroup
			httpClient *mockHTTPClient
		)

		BeforeEach(func() {
			httpClient = &mockHTTPClient{
				lastRequest: make(map[string]*http.Request),
			}
			dtrace.ProxyClient = httpClient
			span = &dtrace.Span{
				System:    "system",
				ID:        "spanid",
				TraceID:   "traceid",
				RawString: "rawstring",
			}
			toProxy = make(chan *dtrace.Span)
			toLoadBalancer = make(chan *dtrace.Span)
			spans = make([]*dtrace.Span, 0)
			wg.Add(1)
			go func(ch chan *dtrace.Span) {
				defer wg.Done()
				for {
					spanReceived, ok := <-ch
					if !ok {
						break
					}
					spans = append(spans, spanReceived)
				}
			}(toLoadBalancer)

			wg.Add(1)
			go func() {
				defer wg.Done()
				dtrace.Proxy(toProxy, toLoadBalancer, config)
			}()
		})

		It("should not proxy request", func() {
			span.TraceHashSum = 1
			toProxy <- span
			close(toProxy)
			wg.Wait()
			Expect(spans).To(HaveLen(1))
		})

		It("should proxy request to replica1", func() {
			span.TraceHashSum = 0
			toProxy <- span
			close(toProxy)
			wg.Wait()
			Expect(spans).To(HaveLen(0))
			Expect(httpClient.requestCount).To(Equal(int32(1)))
			Expect(httpClient.lastRequest["system"].URL.String()).To(Equal("http://replica1:5678/spans?system=system"))
			Expect(httpClient.lastRequest["system"].URL.Path).To(Equal("/spans"))
			body, _ := ioutil.ReadAll(httpClient.lastRequest["system"].Body)
			Expect(string(body)).To(Equal("rawstring\n"))
		})

		It("should proxy request to replica3", func() {
			span.TraceHashSum = 2
			toProxy <- span
			close(toProxy)
			wg.Wait()
			Expect(spans).To(HaveLen(0))
			Expect(httpClient.requestCount).To(Equal(int32(1)))
			Expect(httpClient.lastRequest["system"].URL.String()).To(Equal("https://replica3/spans?system=system"))
		})

		Context("Spans with different systems", func() {
			var span2 *dtrace.Span

			BeforeEach(func() {
				span2 = &dtrace.Span{
					System:    "system2",
					ID:        "spanid2",
					TraceID:   "traceid",
					RawString: "rawstring",
				}
			})

			It("should proxy multiple spans correctly", func() {
				span.TraceHashSum = 2
				span2.TraceHashSum = 2

				toProxy <- span
				toProxy <- span2
				close(toProxy)
				wg.Wait()
				Expect(spans).To(HaveLen(0))
				Expect(httpClient.requestCount).To(Equal(int32(2)))
				Expect(httpClient.lastRequest["system"].URL.String()).To(Equal("https://replica3/spans?system=system"))
			})
		})
	})

	Context("Server", func() {
		var (
			terminate chan os.Signal
			config    = &dtrace.Config{
				Replicas:       []string{"http://replica1:5678", "http://replica2:1234", "https://replica3"},
				MyReplicaIndex: 1,
				Sampling:       1,
				Enable:         true,
				ElasticSearch:  []string{"http://es1:9200", "http://es2:9200"},
				Address:        ":8080",
			}
			wg         sync.WaitGroup
			httpClient *mockHTTPClient
			esClient   *mockESClient
		)

		BeforeEach(func() {
			// hack http to prevent hanging on http.HandleFunc
			http.DefaultServeMux = http.NewServeMux()

			httpClient = &mockHTTPClient{
				lastRequest: make(map[string]*http.Request),
			}
			dtrace.ProxyClient = httpClient
			esClient = &mockESClient{}
			dtrace.ESClientFactory = func(urls []string) (dtrace.ESClient, error) {
				return esClient, nil
			}
			terminate = make(chan os.Signal)
			started := make(chan bool)
			wg.Add(1)
			go func() {
				defer wg.Done()
				dtrace.Listen(terminate, config, started)
			}()
			<-started
		})

		It("should terminate by signal", func() {
			terminate <- os.Kill
			wg.Wait()
		})

		It("should process requests and write document to ES", func() {
			for _, spanJSON := range testPartialJsons {
				http.Post("http://localhost:8080/spans?system=test", "application/json", bufio.NewReader(strings.NewReader(spanJSON)))
			}
			terminate <- os.Kill
			wg.Wait()

			Expect(esClient.bulkRequestCount).To(Equal(int32(1)))
		})

		Context("Bad request", func() {
			var (
				res         *http.Response
				err         error
				invalidJSON string
			)
			AfterEach(func() {
				res, err = http.Post("http://localhost:8080/spans", "application/json", bufio.NewReader(strings.NewReader(invalidJSON)))
				Expect(err).ShouldNot(HaveOccurred())
				Expect(res.StatusCode).To(Equal(400))

				terminate <- os.Kill
				wg.Wait()
			})
			It("should return 400 on invalid json", func() {
				invalidJSON = "{invalid_json}"
			})

			It("should return 400 if system missed", func() {
				invalidJSON = `{"traceId":"1","spanId":"1"}`
			})

			It("should return 400 if traceid missed", func() {
				invalidJSON = `{"system":"1","spanId":"1"}`
			})

			It("should return 400 if spanid missed", func() {
				invalidJSON = `{"system":"1","traceId":"1"}`
			})
		})
	})
})

func makeRequest(body string) *http.Request {
	request, _ := http.NewRequest("POST", "/spans?system=test", bufio.NewReader(strings.NewReader(strings.Replace(body, "\n", "", -1))))
	return request
}

func mustParseTime(value string) time.Time {
	result, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return result
}

func containsPath(chains []*dtrace.Chain, path string) int {
	var count int
	for _, chain := range chains {
		if chain.Path == path {
			count++
		}
	}
	return count
}

func containsPrefix(chains []*dtrace.Chain, prefix string) int {
	var count int
	for _, chain := range chains {
		if chain.Prefix == prefix {
			count++
		}
	}
	return count
}

func fileFeed(filename string, traceMap dtrace.TraceMap) *dtrace.Trace {
	var trace *dtrace.Trace
	file, err := os.Open(filename)
	if err != nil {
		return nil
	}
	reader := bufio.NewReader(file)
	for {
		line, _, err := reader.ReadLine()
		if err == nil {
			if len(strings.Trim(string(line), " ")) > 0 {
				var span dtrace.Span
				errUnmarshal := json.Unmarshal(line, &span)
				Expect(errUnmarshal).ShouldNot(HaveOccurred())
				trace = traceMap.Put(&span)
			}
			continue
		}
		if err != nil && err != io.EOF {
			panic(err)
		}
		break
	}
	return trace
}
