// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package chi_test

import (
	"net/http"

	chitrace "github.com/DataDog/dd-trace-go/contrib/go-chi/chi.v5/v2"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/go-chi/chi/v5"
)

func handler(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("Hello World!\n"))
}

func Example() {
	// Start the tracer
	tracer.Start()
	defer tracer.Stop()

	// Create a chi Router
	router := chi.NewRouter()

	// Use the tracer middleware with the default service name "chi.router".
	router.Use(chitrace.Middleware())

	// Set up some endpoints.
	router.Get("/", handler)

	// And start gathering request traces
	http.ListenAndServe(":8080", router)
}

func Example_withServiceName() {
	// Start the tracer
	tracer.Start()
	defer tracer.Stop()

	// Create a chi Router
	router := chi.NewRouter()

	// Use the tracer middleware with your desired service name.
	router.Use(chitrace.Middleware(chitrace.WithService("chi-server")))

	// Set up some endpoints.
	router.Get("/", handler)

	// And start gathering request traces
	http.ListenAndServe(":8080", router)
}
