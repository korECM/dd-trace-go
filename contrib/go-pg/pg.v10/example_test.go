// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package pg_test

import (
	"log"

	"github.com/go-pg/pg/v10"

	pgtrace "github.com/DataDog/dd-trace-go/contrib/go-pg/pg.v10/v2"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

func Example() {
	tracer.Start()
	defer tracer.Stop()

	conn := pg.Connect(&pg.Options{
		User:     "go-pg-test",
		Database: "datadog",
	})

	// Wrap the connection with the APM hook.
	pgtrace.Wrap(conn)
	var user struct {
		Name string
	}
	_, err := conn.QueryOne(&user, "SELECT name FROM users")
	if err != nil {
		log.Fatal(err)
	}
}
