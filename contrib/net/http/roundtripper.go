// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package http

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"

	"gopkg.in/DataDog/dd-trace-go.v1/appsec/events"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/internal/httptrace"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/net/http/internal/config"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/baggage"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/appsec/emitter/httpsec"
)

type roundTripper struct {
	base http.RoundTripper
	cfg  *roundTripperConfig
}

func (rt *roundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	if rt.cfg.ignoreRequest(req) {
		return rt.base.RoundTrip(req)
	}
	resourceName := rt.cfg.resourceNamer(req)
	spanName := rt.cfg.spanNamer(req)
	// Make a copy of the URL so we don't modify the outgoing request
	url := *req.URL
	url.User = nil // Do not include userinfo in the HTTPURL tag.
	opts := []ddtrace.StartSpanOption{
		tracer.SpanType(ext.SpanTypeHTTP),
		tracer.ResourceName(resourceName),
		tracer.Tag(ext.HTTPMethod, req.Method),
		tracer.Tag(ext.HTTPURL, httptrace.UrlFromRequest(req, rt.cfg.queryString)),
		tracer.Tag(ext.Component, config.ComponentName),
		tracer.Tag(ext.SpanKind, ext.SpanKindClient),
		tracer.Tag(ext.NetworkDestinationName, url.Hostname()),
	}
	if !math.IsNaN(rt.cfg.analyticsRate) {
		opts = append(opts, tracer.Tag(ext.EventSampleRate, rt.cfg.analyticsRate))
	}
	if rt.cfg.serviceName != "" {
		opts = append(opts, tracer.ServiceName(rt.cfg.serviceName))
	}
	if port, err := strconv.Atoi(url.Port()); err == nil {
		opts = append(opts, tracer.Tag(ext.NetworkDestinationPort, port))
	}
	if len(rt.cfg.spanOpts) > 0 {
		opts = append(opts, rt.cfg.spanOpts...)
	}
	span, ctx := tracer.StartSpanFromContext(req.Context(), spanName, opts...)
	defer func() {
		if rt.cfg.after != nil {
			rt.cfg.after(res, span)
		}
		if !events.IsSecurityError(err) && (rt.cfg.errCheck == nil || rt.cfg.errCheck(err)) {
			span.Finish(tracer.WithError(err))
		} else {
			span.Finish()
		}
	}()
	if rt.cfg.before != nil {
		rt.cfg.before(req, span)
	}
	r2 := req.Clone(ctx)
	for k, v := range baggage.All(ctx) {
		span.SetBaggageItem(k, v)
	}
	if rt.cfg.propagation {
		// inject the span context into the http request copy
		err = tracer.Inject(span.Context(), tracer.HTTPHeadersCarrier(r2.Header))
		if err != nil {
			// this should never happen
			fmt.Fprintf(os.Stderr, "contrib/net/http.Roundtrip: failed to inject http headers: %v\n", err)
		}
	}

	if appsec.RASPEnabled() {
		if err := httpsec.ProtectRoundTrip(ctx, r2.URL.String()); err != nil {
			return nil, err
		}
	}

	res, err = rt.base.RoundTrip(r2)
	if err != nil {
		span.SetTag("http.errors", err.Error())
		if rt.cfg.errCheck == nil || rt.cfg.errCheck(err) {
			span.SetTag(ext.Error, err)
		}
	} else {
		span.SetTag(ext.HTTPCode, strconv.Itoa(res.StatusCode))
		if rt.cfg.isStatusError(res.StatusCode) {
			span.SetTag("http.errors", res.Status)
			span.SetTag(ext.Error, fmt.Errorf("%d: %s", res.StatusCode, http.StatusText(res.StatusCode)))
		}
	}
	return res, err
}

// Unwrap returns the original http.RoundTripper.
func (rt *roundTripper) Unwrap() http.RoundTripper {
	return rt.base
}

// WrapRoundTripper returns a new RoundTripper which traces all requests sent
// over the transport.
func WrapRoundTripper(rt http.RoundTripper, opts ...RoundTripperOption) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	cfg := newRoundTripperConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	if wrapped, ok := rt.(*roundTripper); ok {
		rt = wrapped.base
	}
	return &roundTripper{
		base: rt,
		cfg:  cfg,
	}
}

// WrapClient modifies the given client's transport to augment it with tracing and returns it.
func WrapClient(c *http.Client, opts ...RoundTripperOption) *http.Client {
	if c.Transport == nil {
		c.Transport = http.DefaultTransport
	}
	c.Transport = WrapRoundTripper(c.Transport, opts...)
	return c
}
