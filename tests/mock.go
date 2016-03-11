package tests

import (
	"net/http"
	"sync/atomic"
	"sync"

	"github.com/ditrace/ditrace/dtrace"
	"gopkg.in/olivere/elastic.v3"
)

type mockHTTPClient struct {
	requestCount int32
	lastRequest  map[string]*http.Request
	mutex sync.Mutex
}

func (proxyClient *mockHTTPClient) Send(req *http.Request) error {
	proxyClient.mutex.Lock()
	proxyClient.requestCount++
	proxyClient.lastRequest[req.URL.Query()["system"][0]] = req
	proxyClient.mutex.Unlock()
	return nil
}

type mockESClient struct {
	bulkRequestCount int32
}

type mockESBulkService struct {
	client *mockESClient
}

func (b *mockESBulkService) Add(r dtrace.ESBulkRequest) dtrace.ESBulkService {
	return b
}

func (b *mockESBulkService) Do() (*elastic.BulkResponse, error) {
	atomic.AddInt32(&b.client.bulkRequestCount, 1)
	return &elastic.BulkResponse{}, nil
}

type mockESBulkRequest struct {
	index     string
	indexType string
	doc       interface{}
}

func (r *mockESBulkRequest) Index(name string) dtrace.ESBulkRequest {
	r.index = name
	return r
}

func (r *mockESBulkRequest) Type(name string) dtrace.ESBulkRequest {
	r.indexType = name
	return r
}

func (r *mockESBulkRequest) Doc(doc interface{}) dtrace.ESBulkRequest {
	r.doc = doc
	return r
}

func (mockESClient *mockESClient) NewBulkIndexRequest() dtrace.ESBulkRequest {
	return &mockESBulkRequest{}
}

func (mockESClient *mockESClient) Bulk() dtrace.ESBulkService {
	return &mockESBulkService{
		client: mockESClient,
	}
}
