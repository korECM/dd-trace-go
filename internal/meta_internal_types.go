// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package internal

// MetaStructValue is a custom type wrapper used to send metadata to the agent via the `meta_struct` field
// instead of the `meta` inside a span.
type MetaStructValue struct {
	Value any // TODO: further constraining Value's type, especially if it becomes public
}

// TraceSourceTagValue is a custom type wrapper used to create the trace source (_dd.p.ts) tag that will
// be propagated to downstream distributed traces via the `X-Datadog-Tags` HTTP header for example.
// It is represented as a 2 character hexadecimal string
type TraceSourceTagValue struct {
	Value TraceSource
}
