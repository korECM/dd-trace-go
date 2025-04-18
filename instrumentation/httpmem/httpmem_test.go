// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023 Datadog, Inc.

package httpmem_test

import (
	"net/http"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/instrumentation/httpmem"
)

func TestServerAndClient(t *testing.T) {
	s, c := httpmem.ServerAndClient(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer s.Close()
	r, err := http.NewRequest("GET", "http://foo/bar", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
}

func TestServerClosed(t *testing.T) {
	s, c := httpmem.ServerAndClient(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	s.Close()
	r, err := http.NewRequest("GET", "http://foo/bar", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(r)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	t.Fatal("request should have failed, but it succeeded")
}
