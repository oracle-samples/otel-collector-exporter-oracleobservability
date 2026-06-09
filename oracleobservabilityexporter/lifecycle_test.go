// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/loganalytics"
	"go.uber.org/zap"
)

type nopLifecycleWorker struct{}

func (nopLifecycleWorker) sendData(context.Context, []byte) error {
	return nil
}

func init() {
	newLogAnalyticsClientFactory = func(
		AuthenticationType,
		OciConfig,
		string,
		string,
		string,
	) (loganalytics.LogAnalyticsClient, error) {
		return loganalytics.LogAnalyticsClient{}, nil
	}

	newOracleObservabilityWorker = func(
		context.Context,
		*zap.Logger,
		*Config,
		loganalytics.LogAnalyticsClient,
	) oracleobservabilityWorker {
		return nopLifecycleWorker{}
	}
}
