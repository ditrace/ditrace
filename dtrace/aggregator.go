package dtrace

import (
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	hexRe = regexp.MustCompile("(([0-9a-fA-F\\-][0-9a-fA-F\\-]){2}){4,}")
	numbersRe = regexp.MustCompile("\\d+")
)

// Put groups spans by trace ID
func (traceMap TraceMap) Put(span *Span) *Trace {
	trace := traceMap.getOrCreate(span.TraceID)

	trace.Timestamp = time.Now()
	if len(span.ProfileID) > 0 {
		trace.ProfileID = span.ProfileID
	}
	if len(span.System) > 0 {
		trace.System = span.System
	}
	
	span.Revision = span.revision()

	exSpan := trace.Spans.getOrCreate(span.ID)
	if len(span.ParentSpanID) > 0 {
		exSpan.ParentSpanID = span.ParentSpanID
	}

	for key, timestamp := range span.Timeline {
		lowerCaseKey := strings.ToLower(key)
		if existing, exists := exSpan.Timeline[lowerCaseKey]; !exists || existing.Value.Before(timestamp.Value) {
			exSpan.Timeline[lowerCaseKey] = timestamp
		}
	}

	for key, value := range span.Annotations {
		lowerCaseKey := strings.ToLower(key)
		if _, exists := exSpan.Annotations[lowerCaseKey]; !exists || span.isClient() || span.Revision > exSpan.Revision{
			exSpan.Annotations[lowerCaseKey] = value
			if strValue, ok := value.(string); ok {
				lowerCaseValue := strings.ToLower(strValue)
				if lowerCaseKey == "url" {
					exSpan.Annotations["rawurl"] = strValue
					strValue = NormalizeURL(lowerCaseValue)
				}
				exSpan.Annotations[lowerCaseKey] = strValue
			}
		}
	}

	cs := exSpan.getTimestamp("cs")
	cr := exSpan.getTimestamp("cr")
	sr := exSpan.getTimestamp("sr")
	ss := exSpan.getTimestamp("ss")

	if cs != nil && cr != nil {
		exSpan.Annotations["cd"] = cr.Sub(*cs).Nanoseconds() / 1000 // client duration
	}

	if ss != nil && sr != nil {
		exSpan.Annotations["sd"] = ss.Sub(*sr).Nanoseconds() / 1000 // server duration
	}

	if cs != nil && cr != nil && ss != nil && sr != nil {
		exSpan.Annotations["td"] = exSpan.Annotations["cd"].(int64) - exSpan.Annotations["cd"].(int64) // transfer duration
	}

	if len(exSpan.ParentSpanID) == 0 && len(trace.Roots) == 0 {
		trace.Root = exSpan
	}

	if exSpan.IsRoot() {
		trace.Root = exSpan
		trace.Roots[exSpan.ID] = exSpan
	}

	if trace.Root == exSpan {
		trace.Completed = (cs != nil && cr != nil) || (ss != nil && sr != nil)
	} else {
		children, exists := trace.Children[exSpan.ParentSpanID]
		if !exists {
			children = make(SpanMap)
			trace.Children[exSpan.ParentSpanID] = children
		}
		children[exSpan.ID] = exSpan
	}

	return trace
}

// NormalizeURL tries to parse raw url and replace hex sequenses with <hex>
func NormalizeURL(raw string) string {
	var parsed string
	u, err := url.Parse(raw)
	if err == nil {
		parsed = raw
	} else {
		parsed = u.String()
	}
	if len(parsed) > 1 {
		parsed = strings.TrimRight(parsed, "/")
	}
	replaced := hexRe.ReplaceAllString(parsed, "<hex>")
	return numbersRe.ReplaceAllString(replaced, "<num>")
}
