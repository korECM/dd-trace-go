// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package mocktracer

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/ddtrace"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

var _ ddtrace.SpanContext = (*spanContext)(nil)

type spanContext struct {
	sync.RWMutex // guards below fields
	baggage      map[string]string
	priority     int
	hasPriority  bool

	spanID  uint64
	traceID uint64
	span    *tracer.Span
}

func (sc *spanContext) TraceID() string { return strconv.FormatUint(sc.traceID, 10) }

func (sc *spanContext) TraceIDBytes() [16]byte { return [16]byte{} }

func (sc *spanContext) TraceIDLower() uint64 { return sc.traceID }

func (sc *spanContext) SpanID() uint64 { return sc.spanID }

func (sc *spanContext) ForeachBaggageItem(handler func(k, v string) bool) {
	sc.RLock()
	defer sc.RUnlock()
	for k, v := range sc.baggage {
		if !handler(k, v) {
			break
		}
	}
}

func (sc *spanContext) setBaggageItem(k, v string) {
	sc.Lock()
	defer sc.Unlock()
	if sc.baggage == nil {
		sc.baggage = make(map[string]string, 1)
	}
	sc.baggage[k] = v
}

func (sc *spanContext) baggageItem(k string) string {
	sc.RLock()
	defer sc.RUnlock()
	return sc.baggage[k]
}

func (sc *spanContext) setSamplingPriority(p int) {
	sc.Lock()
	defer sc.Unlock()
	sc.priority = p
	sc.hasPriority = true
}

func (sc *spanContext) hasSamplingPriority() bool {
	sc.RLock()
	defer sc.RUnlock()
	return sc.hasPriority
}

func (sc *spanContext) samplingPriority() int {
	sc.RLock()
	defer sc.RUnlock()
	return sc.priority
}

var mockIDSource uint64 = 123

func nextID() uint64 { return atomic.AddUint64(&mockIDSource, 1) }
