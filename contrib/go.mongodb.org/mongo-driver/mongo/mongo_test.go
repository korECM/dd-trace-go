// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package mongo

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/DataDog/dd-trace-go/v2/instrumentation/testutils"
)

func TestMain(m *testing.M) {
	_, ok := os.LookupEnv("INTEGRATION")
	if !ok {
		fmt.Println("--- SKIP: to enable integration test, set the INTEGRATION environment variable")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func Test(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	hostname, port := "localhost", "27017"

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	span, ctx := tracer.StartSpanFromContext(ctx, "mongodb-test")

	addr := fmt.Sprintf("mongodb://localhost:27017/?connect=direct")
	opts := options.Client()
	opts.Monitor = NewMonitor()
	opts.ApplyURI(addr)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.
		Database("test-database").
		Collection("test-collection").
		InsertOne(ctx, bson.D{{Key: "test-item", Value: "test-value"}})
	if err != nil {
		t.Fatal(err)
	}

	span.Finish()

	spans := mt.FinishedSpans()
	assert.Len(t, spans, 2)
	assert.Equal(t, spans[0].TraceID(), spans[1].TraceID())

	s := spans[0]
	assert.Equal(t, ext.SpanTypeMongoDB, s.Tag(ext.SpanType))
	assert.Equal(t, "mongo", s.Tag(ext.ServiceName))
	assert.Equal(t, "mongo.insert", s.Tag(ext.ResourceName))
	assert.Equal(t, hostname, s.Tag(ext.PeerHostname))
	assert.Equal(t, hostname, s.Tag(ext.NetworkDestinationName))
	assert.Equal(t, port, s.Tag(ext.PeerPort))
	assert.Contains(t, s.Tag("mongodb.query"), `"test-item":"test-value"`)
	assert.Equal(t, "test-database", s.Tag(ext.DBInstance))
	assert.Equal(t, "mongo", s.Tag(ext.DBType))
	assert.Equal(t, "go.mongodb.org/mongo-driver/mongo", s.Tag(ext.Component))
	assert.Equal(t, componentName, s.Integration())
	assert.Equal(t, ext.SpanKindClient, s.Tag(ext.SpanKind))
	assert.Equal(t, "mongodb", s.Tag(ext.DBSystem))
}

func TestAnalyticsSettings(t *testing.T) {
	assertRate := func(t *testing.T, mt mocktracer.Tracer, rate interface{}, opts ...Option) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()

		addr := fmt.Sprintf("mongodb://localhost:27017/?connect=direct")
		mongopts := options.Client()
		mongopts.Monitor = NewMonitor(opts...)
		mongopts.ApplyURI(addr)
		client, err := mongo.Connect(ctx, mongopts)
		if err != nil {
			t.Fatal(err)
		}
		client.
			Database("test-database").
			Collection("test-collection").
			InsertOne(ctx, bson.D{{Key: "test-item", Value: "test-value"}})

		spans := mt.FinishedSpans()
		assert.Len(t, spans, 1)
		s := spans[0]
		assert.Equal(t, rate, s.Tag(ext.EventSampleRate))
	}

	t.Run("defaults", func(t *testing.T) {
		mt := mocktracer.Start()
		defer mt.Stop()

		assertRate(t, mt, nil)
	})

	t.Run("enabled", func(t *testing.T) {
		mt := mocktracer.Start()
		defer mt.Stop()

		assertRate(t, mt, 1.0, WithAnalytics(true))
	})

	t.Run("disabled", func(t *testing.T) {
		mt := mocktracer.Start()
		defer mt.Stop()

		assertRate(t, mt, nil, WithAnalytics(false))
	})

	t.Run("override", func(t *testing.T) {
		mt := mocktracer.Start()
		defer mt.Stop()

		testutils.SetGlobalAnalyticsRate(t, 0.4)

		assertRate(t, mt, 0.23, WithAnalyticsRate(0.23))
	})
}
