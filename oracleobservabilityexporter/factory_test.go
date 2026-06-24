// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"context"
	"testing"

	"github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter/internal/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	"go.opentelemetry.io/collector/exporter/exportertest"
)

func TestNewFactory(t *testing.T) {
	t.Parallel()

	factory := NewFactory()

	assert.NotNil(t, factory, "expected a non-nil factory")
	assert.Equal(t, factory.Type().String(), metadata.Type.String(), "expected factory type to be 'oracleobservability'")
}

func TestCreateDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := createDefaultConfig()
	oracleobservabilityConfig := cfg.(*Config)
	oracleobservabilityConfig.NamespaceName = "test-namespace"
	oracleobservabilityConfig.LogGroupID = "test-log-group"

	assert.NotNil(t, cfg, "failed to create default config")
	assert.NoError(t, xconfmap.Validate(cfg))
}

func TestCreateLogsExporter(t *testing.T) {
	ctx := context.Background()
	params := exportertest.NewNopSettings(metadata.Type)
	cfg := createDefaultConfig()

	oracleobservabilityConfig := cfg.(*Config)
	oracleobservabilityConfig.AuthType = "config_file"
	oracleobservabilityConfig.NamespaceName = "test-namespace"
	oracleobservabilityConfig.LogGroupID = "test-log-group"

	exporter, err := createLogsExporter(ctx, params, oracleobservabilityConfig)

	assert.NoError(t, err, "expected no error while creating logs exporter")
	require.NotNil(t, exporter, "expected a non-nil logs exporter")
	require.NoError(t, exporter.Shutdown(context.TODO()))
}

func TestCreateLogsExporterError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	params := exportertest.NewNopSettings(metadata.Type)
	config := &struct{}{} // Invalid config type

	exp, err := createLogsExporter(ctx, params, config)

	expectedErr := "failed to create the logs exporter: invalid configuration type, expected *Config but got *struct {}"
	assert.Error(t, err)
	assert.EqualError(t, err, expectedErr)
	assert.Nil(t, exp)
}
