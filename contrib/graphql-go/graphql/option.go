// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package graphql

import (
	"math"

	internalgraphql "gopkg.in/DataDog/dd-trace-go.v1/contrib/internal/graphql"
	"gopkg.in/DataDog/dd-trace-go.v1/internal"
	"gopkg.in/DataDog/dd-trace-go.v1/internal/namingschema"
)

const defaultServiceName = "graphql.server"

type config struct {
	serviceName   string
	analyticsRate float64
	errExtensions []string
}

type Option func(*config)

func defaults(cfg *config) {
	cfg.serviceName = namingschema.ServiceName(defaultServiceName)
	if internal.BoolEnv("DD_TRACE_GRAPHQL_ANALYTICS_ENABLED", false) {
		cfg.analyticsRate = 1.0
	} else {
		cfg.analyticsRate = math.NaN()
	}
	cfg.errExtensions = internalgraphql.ErrorExtensionsFromEnv()
}

// WithAnalytics enables Trace Analytics for all started spans.
func WithAnalytics(on bool) Option {
	return func(cfg *config) {
		if on {
			cfg.analyticsRate = 1.0
		} else {
			cfg.analyticsRate = math.NaN()
		}
	}
}

// WithAnalyticsRate sets the sampling rate for Trace Analytics events
// correlated to started spans.
func WithAnalyticsRate(rate float64) Option {
	return func(cfg *config) {
		if rate >= 0.0 && rate <= 1.0 {
			cfg.analyticsRate = rate
		} else {
			cfg.analyticsRate = math.NaN()
		}
	}
}

// WithServiceName sets the given service name for the client.
func WithServiceName(name string) Option {
	return func(cfg *config) {
		cfg.serviceName = name
	}
}

// WithErrorExtensions allows to configure the error extensions to include in the error span events.
func WithErrorExtensions(errExtensions ...string) Option {
	return func(cfg *config) {
		cfg.errExtensions = internalgraphql.ParseErrorExtensions(errExtensions)
	}
}
