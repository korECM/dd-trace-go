// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package graphql

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func TestAddErrorsAsSpanEvents(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()
	errExtensions := []string{"ext1", "ext2"}

	type customString string
	type customInt int

	span := tracer.StartSpan("test")
	errs := []Error{
		{
			OriginalErr: errors.New("some error"),
			Message:     "message 1",
			Locations: []ErrorLocation{
				{Line: 1, Column: 2},
				{Line: 100, Column: 200},
			},
			Path: []any{
				"1", 2, "3", 4, customString("5"), customInt(6),
			},
			Extensions: map[string]any{
				"ext1": "ext1",
				"ext2": 2,
				"ext3": 3,
			},
		},
		{
			OriginalErr: errors.New("some error"),
			Message:     "message 2",
			Locations: []ErrorLocation{
				{Line: 2, Column: 3},
				{Line: 200, Column: 300},
			},
			Path: []any{
				"1", 2, "3", 4, customString("5"), customInt(6),
			},
			Extensions: map[string]any{
				"ext1": "ext1",
				"ext3": 3,
			},
		},
	}
	AddErrorsAsSpanEvents(span, errs, errExtensions)
	span.Finish()

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	events := spans[0].Events()
	require.Len(t, events, 2)

	assert.Equal(t, "dd.graphql.query.error", events[0].Name)
	assert.NotEmpty(t, events[0].Config.Time)

	assert.NotEmpty(t, events[0].Config.Attributes["stacktrace"])
	wantAttrs1 := map[string]any{
		"message":         "message 1",
		"type":            "*errors.errorString",
		"location":        []string{"1:2", "100:200"},
		"stacktrace":      events[0].Config.Attributes["stacktrace"],
		"path":            []string{"1", "2", "3", "4", "5", "6"},
		"extensions.ext1": "ext1",
		"extensions.ext2": 2,
	}
	assert.Equal(t, wantAttrs1, events[0].Config.Attributes)

	assert.Equal(t, "dd.graphql.query.error", events[1].Name)
	assert.NotEmpty(t, events[1].Config.Time)

	assert.NotEmpty(t, events[1].Config.Attributes["stacktrace"])
	wantAttrs2 := map[string]any{
		"message":         "message 2",
		"type":            "*errors.errorString",
		"location":        []string{"2:3", "200:300"},
		"stacktrace":      events[1].Config.Attributes["stacktrace"],
		"path":            []string{"1", "2", "3", "4", "5", "6"},
		"extensions.ext1": "ext1",
	}
	assert.Equal(t, wantAttrs2, events[1].Config.Attributes)
}
