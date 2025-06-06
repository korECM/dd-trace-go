// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package pubsub_test

import (
	"context"
	"log"

	"cloud.google.com/go/pubsub"

	pubsubtrace "github.com/DataDog/dd-trace-go/contrib/cloud.google.com/go/pubsub.v1/v2"
)

func ExamplePublish() {
	client, err := pubsub.NewClient(context.Background(), "project-id")
	if err != nil {
		log.Fatal(err)
	}

	topic := client.Topic("topic")
	_, err = pubsubtrace.Publish(context.Background(), topic, &pubsub.Message{Data: []byte("hello world!")}).Get(context.Background())
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleSubscription_Receive() {
	client, err := pubsub.NewClient(context.Background(), "project-id")
	if err != nil {
		log.Fatal(err)
	}

	sub := client.Subscription("subscription")
	err = sub.Receive(context.Background(), pubsubtrace.WrapReceiveHandler(sub, func(_ context.Context, _ *pubsub.Message) {
		// TODO: Handle message.
	}))
	if err != nil {
		log.Fatal(err)
	}
}
