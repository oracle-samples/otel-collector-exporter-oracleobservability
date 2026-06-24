// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter/internal/metadata"
	oci_common "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

// Mock worker for sending data
type mockWorker struct {
	mock.Mock
}

func (m *mockWorker) sendData(ctx context.Context, data []byte) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

type stubOCIResponse struct {
	statusCode int
}

func (s stubOCIResponse) HTTPResponse() *http.Response {
	return &http.Response{StatusCode: s.statusCode}
}

func TestNewLogsExporter_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	params := exportertest.NewNopSettings(metadata.Type)
	cfg := &Config{
		AuthType:      ConfigFile,
		NamespaceName: "test-namespace",
		LogGroupID:    "test-log-group-id",
	}

	exp, err := newLogsExporter(ctx, params, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, exp)
}

func TestPushLogData_EmptyLogs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()
	mockWorker := new(mockWorker)
	exp := &oracleobservabilityLogsExporter{
		logger:                    logger,
		oracleobservabilityWorker: mockWorker,
	}

	logs := plog.NewLogs()

	err := exp.pushLogData(ctx, logs)
	assert.NoError(t, err)
}

func TestPushLogData_SingleBatch_Success(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	mockWorker := new(mockWorker)
	exp := &oracleobservabilityLogsExporter{
		logger:                    logger,
		oracleobservabilityWorker: mockWorker,
	}

	ctx := context.Background()
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	logRecord := sl.LogRecords().AppendEmpty()
	logRecord.Body().SetStr("Test log entry")

	mockWorker.On("sendData", mock.Anything, mock.Anything).Return(nil)

	err := exp.pushLogData(ctx, logs)
	assert.NoError(t, err)
	mockWorker.AssertExpectations(t)
}

func TestPushLogData_WorkerNotInitialized(t *testing.T) {
	t.Parallel()

	exp := &oracleobservabilityLogsExporter{
		logger: zap.NewNop(),
	}

	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	sl.LogRecords().AppendEmpty().Body().SetStr("test log")

	err := exp.pushLogData(context.Background(), logs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "worker is not initialized")
}

func TestChunkPlogs_Truncate(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	mockWorker := new(mockWorker)
	exp := &oracleobservabilityLogsExporter{
		logger:                    logger,
		oracleobservabilityWorker: mockWorker,
	}

	ctx := context.Background()
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	logRecord := sl.LogRecords().AppendEmpty()
	logRecord.Body().SetStr(string(make([]byte, MaxContentLengthLogsInBytes+100))) // Large log entry

	mockWorker.On("sendData", mock.Anything, mock.Anything).Return(nil)

	err := exp.pushLogData(ctx, logs)
	assert.NoError(t, err)
	mockWorker.AssertExpectations(t)
}

func TestChunkPlogs_Truncate_DoesNotExceedMaxChunkSize(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	mockWorker := new(mockWorker)
	exp := &oracleobservabilityLogsExporter{
		logger:                    logger,
		oracleobservabilityWorker: mockWorker,
	}

	ctx := context.Background()
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.Body().SetStr(strings.Repeat("a", MaxContentLengthLogsInBytes*2))

	calls := 0
	hasTruncatedSuffix := false

	mockWorker.On("sendData", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		payload := args.Get(1).([]byte)
		calls++
		require.LessOrEqual(t, len(payload), MaxContentLengthLogsInBytes)
		if bytes.Contains(payload, []byte(truncationSuffix)) {
			hasTruncatedSuffix = true
		}
	})

	err := exp.pushLogData(ctx, logs)
	require.NoError(t, err)
	require.Greater(t, calls, 0)
	require.True(t, hasTruncatedSuffix)
	mockWorker.AssertExpectations(t)
}

func TestChunkPlogs_2RL(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	mockWorker := new(mockWorker)
	exp := &oracleobservabilityLogsExporter{
		logger:                    logger,
		oracleobservabilityWorker: mockWorker,
	}

	ctx := context.Background()
	logs := plog.NewLogs()

	rl1 := logs.ResourceLogs().AppendEmpty()
	rl2 := logs.ResourceLogs().AppendEmpty()

	rl1.Resource().Attributes().PutStr("resource-attr", "resource-attr-val-1")
	rl2.Resource().Attributes().PutStr("resource-attr", "resource-attr-val-1")

	sl1_1 := rl1.ScopeLogs().AppendEmpty()

	lr1_1 := sl1_1.LogRecords().AppendEmpty()
	lr1_1.SetTimestamp(pcommon.Timestamp(1735809312000000000))
	lr1_1.SetSeverityNumber(plog.SeverityNumberInfo)
	lr1_1.SetSeverityText("Info")
	lr1_1.Body().SetStr(string(make([]byte, 1000)))
	lr1_1.SetDroppedAttributesCount(1)
	lr1_1.SetTraceID(pcommon.TraceID([16]byte{0x08, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}))
	lr1_1.SetSpanID(pcommon.SpanID([8]byte{0x01, 0x02, 0x04, 0x08, 0x00, 0x00, 0x00, 0x00}))

	sl2_1 := rl2.ScopeLogs().AppendEmpty()

	lr2_1 := sl2_1.LogRecords().AppendEmpty()
	lr2_1.SetTimestamp(pcommon.Timestamp(1735809312000000000))
	lr2_1.SetSeverityNumber(plog.SeverityNumberInfo)
	lr2_1.SetSeverityText("Info")
	lr2_1.Body().SetStr(string(make([]byte, 1000)))
	lr2_1.SetDroppedAttributesCount(1)
	lr2_1.SetTraceID(pcommon.TraceID([16]byte{0x08, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}))
	lr2_1.SetSpanID(pcommon.SpanID([8]byte{0x01, 0x02, 0x04, 0x08, 0x00, 0x00, 0x00, 0x00}))

	mockWorker.On("sendData", mock.Anything, mock.Anything).Return(nil)

	err := exp.pushLogData(ctx, logs)
	assert.NoError(t, err)
	mockWorker.AssertExpectations(t)
}

func TestChunkPlogs_1RL_2SL_3LR(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	mockWorker := new(mockWorker)
	exp := &oracleobservabilityLogsExporter{
		logger:                    logger,
		oracleobservabilityWorker: mockWorker,
	}

	ctx := context.Background()
	logs := plog.NewLogs()

	rl := logs.ResourceLogs().AppendEmpty()

	rl.Resource().Attributes().PutStr("resource-attr", "resource-attr-val-1")

	sl1 := rl.ScopeLogs().AppendEmpty()
	sl2 := rl.ScopeLogs().AppendEmpty()

	lr1 := sl1.LogRecords().AppendEmpty()
	lr1.SetSeverityNumber(plog.SeverityNumberInfo)
	lr1.SetSeverityText("Info")
	lr1.Body().SetStr(string(make([]byte, 1000)))
	lr1.SetDroppedAttributesCount(1)
	lr1.SetTraceID(pcommon.TraceID([16]byte{0x08, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}))
	lr1.SetSpanID(pcommon.SpanID([8]byte{0x01, 0x02, 0x04, 0x08, 0x00, 0x00, 0x00, 0x00}))

	lr2 := sl2.LogRecords().AppendEmpty()
	lr2.SetSeverityNumber(plog.SeverityNumberInfo)
	lr2.SetSeverityText("Info")
	lr2.Body().SetStr(string(make([]byte, 1000)))
	lr2.SetDroppedAttributesCount(1)
	lr2.SetTraceID(pcommon.TraceID([16]byte{0x08, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}))
	lr2.SetSpanID(pcommon.SpanID([8]byte{0x01, 0x02, 0x04, 0x08, 0x00, 0x00, 0x00, 0x00}))

	lr3 := sl2.LogRecords().AppendEmpty()
	lr3.SetSeverityNumber(plog.SeverityNumberInfo)
	lr3.SetSeverityText("Info")
	lr3.Body().SetStr(string(make([]byte, 1000)))
	lr3.SetDroppedAttributesCount(1)
	lr3.SetTraceID(pcommon.TraceID([16]byte{0x08, 0x04, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}))
	lr3.SetSpanID(pcommon.SpanID([8]byte{0x01, 0x02, 0x04, 0x08, 0x00, 0x00, 0x00, 0x00}))

	mockWorker.On("sendData", mock.Anything, mock.Anything).Return(nil)

	err := exp.pushLogData(ctx, logs)
	assert.NoError(t, err)
	mockWorker.AssertExpectations(t)
}

func mockZapProduction() (*zap.Logger, error) {
	return nil, errors.New("failed to create logger")
}

func TestNewLogsExporter_FailedLoggerInitialization(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	params := exportertest.NewNopSettings(metadata.Type)
	cfg := &Config{}

	logger, err := mockZapProduction()
	assert.Error(t, err)

	logExporter, err := newLogsExporterWithLogger(ctx, params, cfg, logger)

	assert.Nil(t, logExporter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize zap logger")
}

func newLogsExporterWithLogger(ctx context.Context, params exporter.Settings, cfg *Config, logger *zap.Logger) (*oracleobservabilityLogsExporter, error) {
	if logger == nil {
		return nil, errors.New("failed to initialize zap logger")
	}

	return &oracleobservabilityLogsExporter{
		logger:                    logger,
		config:                    cfg,
		cancel:                    func() {},
		oracleobservabilityWorker: &defaultWorker{},
	}, nil
}

func TestNewLogsExporter_InvalidConfiguration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	params := exportertest.NewNopSettings(metadata.Type)
	invalidCfg := struct{}{}

	logExporter, err := newLogsExporter(ctx, params, invalidCfg)

	assert.Nil(t, logExporter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid configuration type")
}

func TestNewLogsExporter_ValidationErrorDoesNotInitializeClient(t *testing.T) {
	ctx := context.Background()
	params := exportertest.NewNopSettings(metadata.Type)
	cfg := &Config{
		AuthType:   ConfigFile,
		LogGroupID: "test-log-group-id",
	}

	logExporter, err := newLogsExporter(ctx, params, cfg)
	require.Nil(t, logExporter)
	require.EqualError(t, err, "'namespace' is a required field. You may find using OCI Console under Log Analytics → Administration → Service")
}

func TestInitializeOciLogAnalyticsClient_InvalidAuthType(t *testing.T) {
	t.Parallel()

	client, err := initializeOciLogAnalyticsClient("invalid", OciConfig{}, "", "", "")

	require.Error(t, err)
	require.Contains(t, err.Error(), `unsupported auth_type "invalid"`)
	require.Empty(t, client)
}

func TestStart(t *testing.T) {
	t.Parallel()

	exp := &oracleobservabilityLogsExporter{
		logger:                    zap.NewNop(),
		config:                    &Config{},
		cancel:                    func() {},
		oracleobservabilityWorker: &defaultWorker{},
	}

	err := exp.Start(context.Background(), componenttest.NewNopHost())
	assert.NoError(t, err)
	assert.NotNil(t, exp.oracleobservabilityWorker)
}

func TestShutdown(t *testing.T) {
	t.Parallel()

	exp := &oracleobservabilityLogsExporter{
		logger:                    zap.NewNop(),
		config:                    &Config{},
		cancel:                    func() {},
		oracleobservabilityWorker: &defaultWorker{},
	}

	err := exp.Shutdown(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, exp.oracleobservabilityWorker)
}

func TestCalculateResourceLogsCount(t *testing.T) {
	t.Parallel()

	resourceLogSizes := []int{100, 200, 300, 400, 500}
	availableBufferBytes := 750
	count := calculateResourceLogsCount(resourceLogSizes, 0, &availableBufferBytes)
	assert.Equal(t, 3, count)
	assert.Equal(t, 150, availableBufferBytes)
}

func TestCalculateScopeLogsCount(t *testing.T) {
	t.Parallel()

	scopeLogsSize := [][]int{{50, 100, 150}, {200, 250, 300}}
	availableBufferBytes := 300
	count := calculateScopeLogsCount(scopeLogsSize, 0, 0, &availableBufferBytes)
	assert.Equal(t, 3, count)
	assert.Equal(t, 0, availableBufferBytes)
}

func TestCalculateLogRecordsCount(t *testing.T) {
	t.Parallel()

	logRecordsSize := [][][]int{{{10, 20, 30}, {40, 50, 60}}, {{70, 80, 90}, {100, 110, 120}}}
	availableBufferBytes := 100
	count := calculateLogRecordsCount(logRecordsSize, 0, 0, 0, &availableBufferBytes)
	assert.Equal(t, 3, count)
	assert.Equal(t, 40, availableBufferBytes)
}

func TestShouldRetryOnNon2xxResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resp     oci_common.OCIOperationResponse
		expected bool
	}{
		{
			name: "Retry when OCI error exists",
			resp: oci_common.OCIOperationResponse{
				Error: errors.New("transient error"),
			},
			expected: true,
		},
		{
			name: "Retry when response is nil",
			resp: oci_common.OCIOperationResponse{
				Response: nil,
			},
			expected: true,
		},
		{
			name: "Do not retry on 2xx",
			resp: oci_common.OCIOperationResponse{
				Response: stubOCIResponse{statusCode: http.StatusNoContent},
			},
			expected: false,
		},
		{
			name: "Retry on non-2xx",
			resp: oci_common.OCIOperationResponse{
				Response: stubOCIResponse{statusCode: http.StatusBadGateway},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := shouldRetryOnNon2xxResponse(tt.resp)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
