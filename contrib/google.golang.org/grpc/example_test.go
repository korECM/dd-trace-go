// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package grpc_test

import (
	"log"
	"net"

	grpctrace "github.com/DataDog/dd-trace-go/contrib/google.golang.org/grpc/v2"

	"google.golang.org/grpc"
)

func Example_client() {
	// Create the client interceptor using the grpc trace package.
	si := grpctrace.StreamClientInterceptor(grpctrace.WithService("my-grpc-client"))
	ui := grpctrace.UnaryClientInterceptor(grpctrace.WithService("my-grpc-client"))

	// Dial in using the created interceptor.
	// Note: To use multiple UnaryInterceptors with grpc.Dial, you must use
	// grpc.WithChainUnaryInterceptor instead (as of google.golang.org/grpc v1.51.0).
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure(),
		grpc.WithStreamInterceptor(si), grpc.WithUnaryInterceptor(ui))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// And continue using the connection as normal.
}

func Example_server() {
	// Create a listener for the server.
	ln, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}

	// Create the server interceptor using the grpc trace package.
	si := grpctrace.StreamServerInterceptor(grpctrace.WithService("my-grpc-server"))
	ui := grpctrace.UnaryServerInterceptor(grpctrace.WithService("my-grpc-server"))

	// Initialize the grpc server as normal, using the tracing interceptor.
	s := grpc.NewServer(grpc.StreamInterceptor(si), grpc.UnaryInterceptor(ui))

	// ... register your services

	// Start serving incoming connections.
	if err := s.Serve(ln); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
