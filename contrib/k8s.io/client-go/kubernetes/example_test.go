// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package kubernetes_test

import (
	"context"
	"fmt"

	kubernetestrace "github.com/DataDog/dd-trace-go/contrib/k8s.io/client-go/v2/kubernetes"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
)

func Example() {
	tracer.Start()
	defer tracer.Stop()

	cfg, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// Use this to trace all calls made to the Kubernetes API
	cfg.WrapTransport = kubernetestrace.WrapRoundTripper

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err.Error())
	}

	pods, err := client.CoreV1().Pods("default").List(context.TODO(), meta_v1.ListOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Println(pods.Items)
}
