// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

// Package fiber provides tracing functions for tracing the fiber package (https://github.com/gofiber/fiber).
package fiber // import "github.com/DataDog/dd-trace-go/contrib/gofiber/fiber.v2/v2"

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/DataDog/dd-trace-go/v2/instrumentation"

	"github.com/gofiber/fiber/v2"
)

const componentName = "gofiber/fiber.v2"

var instr *instrumentation.Instrumentation

func init() {
	instr = instrumentation.Load(instrumentation.PackageGoFiberV2)
}

// Middleware returns middleware that will trace incoming requests.
func Middleware(opts ...Option) func(c *fiber.Ctx) error {
	cfg := new(config)
	defaults(cfg)
	for _, fn := range opts {
		fn.apply(cfg)
	}
	instr.Logger().Debug("gofiber/fiber.v2: Middleware: %#v", cfg)
	return func(c *fiber.Ctx) error {
		if cfg.ignoreRequest(c) {
			return c.Next()
		}

		opts := []tracer.StartSpanOption{
			tracer.SpanType(ext.SpanTypeWeb),
			tracer.ServiceName(cfg.serviceName),
			tracer.Tag(ext.HTTPMethod, c.Method()),
			tracer.Tag(ext.HTTPURL, string(c.Request().URI().PathOriginal())),
			tracer.Measured(),
		}
		if !math.IsNaN(cfg.analyticsRate) {
			opts = append(opts, tracer.Tag(ext.EventSampleRate, cfg.analyticsRate))
		}
		// Create a http.Header object so that a parent trace can be extracted. Fiber uses a non-standard header carrier
		h := http.Header{}
		for k, headers := range c.GetReqHeaders() {
			for _, v := range headers {
				// GetReqHeaders returns a list of headers associated with the given key.
				// http.Header.Add supports appending multiple values, so the previous
				// value will not be overwritten.
				h.Add(k, v)
			}
		}
		if spanctx, err := tracer.Extract(tracer.HTTPHeadersCarrier(h)); err == nil {
			// If there are span links as a result of context extraction, add them as a StartSpanOption
			if spanctx != nil && spanctx.SpanLinks() != nil {
				opts = append(opts, tracer.WithSpanLinks(spanctx.SpanLinks()))
			}
			opts = append(opts, tracer.ChildOf(spanctx))
		}
		opts = append(opts, cfg.spanOpts...)
		opts = append(opts,
			tracer.Tag(ext.Component, componentName),
			tracer.Tag(ext.SpanKind, ext.SpanKindServer),
		)
		span, ctx := tracer.StartSpanFromContext(c.UserContext(), cfg.spanName, opts...)

		defer span.Finish()

		// pass the span through the request UserContext
		c.SetUserContext(ctx)

		// pass the execution down the line
		err := c.Next()

		span.SetTag(ext.ResourceName, cfg.resourceNamer(c))
		span.SetTag(ext.HTTPRoute, c.Route().Path)

		status := c.Response().StatusCode()
		// on the off chance we don't yet have a status after the rest of the things have run
		if status == 0 {
			// 0 - means we do not have a status code at this point
			// in case the response was returned by a middleware without one
			status = http.StatusOK
		}
		span.SetTag(ext.HTTPCode, strconv.Itoa(status))

		if err != nil {
			span.SetTag(ext.Error, err)
		} else if cfg.isStatusError(status) {
			// mark 5xx server error
			span.SetTag(ext.Error, fmt.Errorf("%d: %s", status, http.StatusText(status)))
		}
		return err
	}
}
