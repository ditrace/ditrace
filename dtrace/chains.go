package dtrace

import (
	"fmt"
	"strings"
)

type chainLevel struct {
	Children []string
	Chain    []string
	Prefix   []string
	SpanID   string
	Level    int
}

// GetChains returns chain list of trace
func (trace *Trace) GetChains(rootSpanID string) ([]*Span, []*Chain, error) {
	var (
		stack  []*chainLevel
		spans  []*Span
		chains []*Chain
	)
	stack = append(stack, &chainLevel{
		Children: []string{rootSpanID},
		Chain:    make([]string, 0),
		Prefix:   make([]string, 0),
	})

	for len(stack) > 0 {
		item := stack[0]
		stack = stack[1:]
		if item.Level > 255 {
			return nil, nil, fmt.Errorf("Too long stack")
		}
		var itemSpan *Span
		if len(item.SpanID) != 0 {
			itemSpan = trace.Spans[item.SpanID]
			trace.patchTargetID(itemSpan)
			itemSpan.Prefix = getPath(trace.Spans, item.Prefix)
			spans = append(spans, itemSpan)
		}
		var (
			levelIncrement int
			prefixAddition []string
			getChain       func(*chainLevel) []string
		)

		if len(item.Children) == 1 && item.SpanID != rootSpanID {
			levelIncrement = 0
			prefixAddition = []string{}
			getChain = func(chainLevel *chainLevel) []string {
				return chainLevel.Chain
			}
		} else if len(item.SpanID) != 0 {
			chain := &Chain{
				Path:   getPath(trace.Spans, item.Chain),
				Prefix: getPath(trace.Spans, item.Prefix),
				Level:  item.Level,
			}
			chain.Duration = itemSpan.Annotations.get(int64(0), "cd", "sd").(int64)
			chains = append(chains, chain)
			levelIncrement = 1
			prefixAddition = item.Chain
			getChain = func(chainLevel *chainLevel) []string {
				return []string{chainLevel.Chain[(len(chainLevel.Chain) - 1)]}
			}
		}

		if itemSpan != nil && itemSpan.IsRoot() && itemSpan.ID != rootSpanID {
			continue
		}

		if getChain != nil {
			for _, spanID := range item.Children {
				chain := getChain(item)
				chain = trace.appendChain(chain, spanID)
				stack = append(stack, &chainLevel{
					Children: trace.getChildren(spanID),
					Chain:    chain,
					Prefix:   append(item.Prefix, prefixAddition...),
					SpanID:   spanID,
					Level:    item.Level + levelIncrement,
				})
			}
		}

	}
	return spans, chains, nil
}

func getPath(spanMap SpanMap, spanIDs []string) string {
	targets := make([]string, 0, len(spanIDs))
	for _, spanID := range spanIDs {
		targets = append(targets, spanMap[spanID].Annotations["targetid"].(string))
	}
	return strings.Join(targets, "->")
}

func (trace *Trace) appendChain(chain []string, spanID string) []string {
	if len(chain) > 0 {
		if _, exists := trace.Spans[chain[len(chain)-1]].Annotations["wrapper"]; exists {
			chain[len(chain)-1] = spanID
			return chain
		}
	}
	chain = append(chain, spanID)
	return chain
}
