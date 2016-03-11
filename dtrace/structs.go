package dtrace

import (
	"hash/fnv"
	"time"
)

// Span struct
type Span struct {
	System       string
	TraceID      string
	ProfileID    string
	ParentSpanID string
	ID           string `json:"spanid"`
	Annotations  SpanAnnotations
	Timeline     SpanTimeline
	Revision     int
	Prefix       string
	TraceHashSum uint64
	RawString    string
}

// SpanTimestamp struct
type SpanTimestamp struct {
	Value time.Time
}

// UnmarshalJSON implements json.Unmarshaler interface
func (st *SpanTimestamp) UnmarshalJSON(bytes []byte) (err error) {
	if bytes[0] == '"' {
		bytes = bytes[1:]
	}
	if bytes[len(bytes)-1] == '"' {
		bytes = bytes[0 : len(bytes)-1]
	}
	if bytes[len(bytes)-1] != 'Z' {
		bytes = append(bytes, 'Z')
	}
	st.Value, err = time.Parse(time.RFC3339Nano, string(bytes))
	return err
}

// SpanAnnotations is map[string]interface{}
type SpanAnnotations map[string]interface{}

// SpanTimeline is map[string]SpanTimestamp
type SpanTimeline map[string]SpanTimestamp

// Trace struct
type Trace struct {
	ID        string
	Roots     SpanMap
	Children  map[string]SpanMap
	Spans     SpanMap
	Timestamp time.Time
	System    string
	ProfileID string
	Root      *Span
	Completed bool
}

// Chain struct
type Chain struct {
	Path     string `json:"path"`
	Prefix   string `json:"prefix"`
	Level    int    `json:"level"`
	Duration int64  `json:"duration"`
}

// TraceMap is map[string]*Trace
type TraceMap map[string]*Trace

func (traceMap TraceMap) getOrCreate(traceid string) *Trace {

	trace, exists := traceMap[traceid]
	if !exists {
		trace = &Trace{
			ID:       traceid,
			Roots:    make(SpanMap),
			Children: make(map[string]SpanMap),
			Spans:    make(SpanMap),
		}
		traceMap[traceid] = trace
	}
	return trace
}

// SpanMap is map[string]*Span
type SpanMap map[string]*Span

func (spanMap SpanMap) getOrCreate(spanid string) *Span {

	span, exists := spanMap[spanid]
	if !exists {
		span = &Span{
			ID:          spanid,
			Timeline:    make(SpanTimeline),
			Annotations: make(SpanAnnotations),
		}
		spanMap[spanid] = span
	}
	return span
}

func (trace *Trace) getChildren(spanID string) []string {
	var spanIDs []string
	if children, exists := trace.Children[spanID]; exists {
		spanIDs = make([]string, 0, len(children))
		for id := range children {
			spanIDs = append(spanIDs, id)
		}
	}
	return spanIDs
}

func (span *Span) revision() int {
	return int(span.Annotations.get(float64(0), "revision").(float64))
}

func (span *Span) isClient() bool {
	if _, exists := span.Timeline["cs"]; exists {
		return true
	}
	if _, exists := span.Timeline["cr"]; exists {
		return true
	}
	return false
}

// CalculateTraceHashSum calculates hash sum for trace id
func (span *Span) CalculateTraceHashSum() {
	h := fnv.New64()
	h.Write([]byte(span.TraceID))
	hashSum := h.Sum64()
	span.TraceHashSum = hashSum
}

// IsRoot returns true if span contains root annotation
func (span *Span) IsRoot() bool {
	_, exists := span.Annotations["root"]
	return exists
}

func (span *Span) getTimestamp(name string) *time.Time {
	if timestamp, exists := span.Timeline[name]; exists {
		return &timestamp.Value
	}
	return nil
}

func (trace *Trace) patchTargetID(span *Span) {
	if _, exists := span.Annotations["targetid"]; !exists {
		span.Annotations["targetid"] = span.Annotations.get("unknown", "host")
	}
	if len(span.ParentSpanID) == 0 {
		return
	}
	parentSpan, exists := trace.Spans[span.ParentSpanID]
	if !exists {
		return
	}
	if _, exists := parentSpan.Annotations["wrapper"]; !exists {
		return
	}
	if targetid, exists := parentSpan.Annotations["targetid"]; exists {
		span.Annotations["targetid"] = targetid
	}
}

func (annotations SpanAnnotations) get(defaultValue interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, exists := annotations[key]; exists {
			return value
		}
	}
	return defaultValue
}

func (timeline SpanTimeline) get(defaultValue time.Time, keys ...string) time.Time {
	for _, key := range keys {
		if timestamp, exists := timeline[key]; exists {
			return timestamp.Value
		}
	}
	return defaultValue
}
