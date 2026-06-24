// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"context"
	"fmt"
	"time"

	"github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		metadata.Type,
		createDefaultConfig,
		exporter.WithLogs(createLogsExporter, metadata.LogsStability),
	)
}

func createDefaultConfig() component.Config {
	cfg := &Config{
		BackOffConfig: configretry.BackOffConfig{
			Enabled:             true,
			InitialInterval:     5 * time.Second,
			RandomizationFactor: 0.5,
			Multiplier:          1.5,
			MaxInterval:         30 * time.Second,
			MaxElapsedTime:      0,
		},
		QueueConfig: configoptional.Some(exporterhelper.QueueBatchConfig{
			NumConsumers:    10,
			QueueSize:       1000,
			BlockOnOverflow: true,
			WaitForResult:   false,
			Sizer:           exporterhelper.RequestSizerTypeRequests,
		}),
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 0,
		},
		AuthType: ConfigFile,
	}

	return cfg
}

func createLogsExporter(
	ctx context.Context,
	params exporter.Settings,
	cfg component.Config,
) (exporter.Logs, error) {
	exp, err := newLogsExporter(ctx, params, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create the logs exporter: %v", err)
	}
	return exp, nil
}
