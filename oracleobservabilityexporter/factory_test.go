// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"context"
	"testing"

	"github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter/internal/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
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
	assert.NoError(t, confmap.Validate(cfg))
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

func TestQueueStorageDefaults(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		values  map[string]any
		storage string
		enabled bool
	}{
		{"omitted", map[string]any{}, "file_storage", true},
		{"partial queue", map[string]any{"sending_queue": map[string]any{"queue_size": 42}}, "file_storage", true},
		{"custom storage", map[string]any{"sending_queue": map[string]any{"storage": "file_storage/custom"}}, "file_storage/custom", true},
		{"disabled", map[string]any{"sending_queue": map[string]any{"enabled": false}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := createDefaultConfig().(*Config)
			require.NoError(t, confmap.NewFromStringMap(tc.values).Unmarshal(cfg))
			require.Equal(t, tc.enabled, cfg.QueueConfig.HasValue())
			if tc.enabled {
				require.NotNil(t, cfg.QueueConfig.Get().StorageID)
				assert.Equal(t, tc.storage, cfg.QueueConfig.Get().StorageID.String())
			}
		})
	}
	first := createDefaultConfig().(*Config)
	*first.QueueConfig.Get().StorageID = component.NewIDWithName(component.MustNewType("file_storage"), "changed")
	second := createDefaultConfig().(*Config)
	assert.Equal(t, "file_storage", second.QueueConfig.Get().StorageID.String())
}

func TestDefaultQueueRequiresStorageExtension(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.NamespaceName = "test-namespace"
	cfg.LogGroupID = "test-log-group"
	exp, err := createLogsExporter(context.Background(), exportertest.NewNopSettings(metadata.Type), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, exp.Shutdown(context.Background())) })
	require.ErrorContains(t, exp.Start(context.Background(), componenttest.NewNopHost()), "no storage client extension found")
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
