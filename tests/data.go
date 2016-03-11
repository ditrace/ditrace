package tests

var testPartialJsons = []string{`{"traceId":"432fa4b867ef41f882f6f575b0b1e180","spanId":"4ee8c1d5","parentSpanId":"f51f8e99-adbd-4b86-915e-d2175c5b29d6","annotations":{"targetId":"Queue"},"timeline":{"sr":"2016-03-03T05:38:05.9018695Z"}}`,
	`{"traceId":"432fa4b867ef41f882f6f575b0b1e180","spanId":"f51f8e99-adbd-4b86-915e-d2175c5b29d6","parentSpanId":"f71982ea","annotations":{"targetId":"TaskLife"},"timeline":{"sr":"2016-03-03T05:38:05.9018339Z"}}`,
	`{"traceId":"432fa4b867ef41f882f6f575b0b1e180","spanId":"f71982ea","annotations":{"url":"http://localhost:12345/sendmessage/"},"timeline":{"sr":"2016-03-03T05:38:05.8618078Z","ss":"2016-03-03T05:38:05.9278514Z"}}`,
	`{"traceId":"432fa4b867ef41f882f6f575b0b1e180","spanId":"f71982ea","annotations":{"url":"http://localhost:12345/sendmessage/","targetId":"Client-Server","root":"true"},"timeline":{"cs":"2016-03-03T05:38:05.6086412Z","cr":"2016-03-03T05:38:05.9528661Z"}}`,
	`{"traceId":"432fa4b867ef41f882f6f575b0b1e180","spanId":"4ee8c1d5","annotations":{},"timeline":{"ss":"2016-03-03T05:38:05.9538681Z"}}`,
	`{"traceId":"432fa4b867ef41f882f6f575b0b1e180","spanId":"4d27b43e","parentSpanId":"f51f8e99-adbd-4b86-915e-d2175c5b29d6","annotations":{"targetId":"Handling"},"timeline":{"sr":"2016-03-03T05:38:05.9538750Z","ss":"2016-03-03T05:38:06.1562328Z"}}`,
	`{"traceId":"432fa4b867ef41f882f6f575b0b1e180","spanId":"f51f8e99-adbd-4b86-915e-d2175c5b29d6","annotations":{},"timeline":{"ss":"2016-03-03T05:38:06.1562424Z"}}`,
}
