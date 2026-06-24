// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestEstimateSizes(t *testing.T) {
	t.Parallel()

	logs := plog.NewLogs()

	resourceLog := logs.ResourceLogs().AppendEmpty()
	resourceLog.Resource().Attributes().PutStr("key1", "value1")

	scopeLog := resourceLog.ScopeLogs().AppendEmpty()
	scopeLog.Scope().SetName("test-scope")
	scopeLog.Scope().SetVersion("v1.0.0")
	scopeLog.Scope().Attributes().PutInt("attr1", 100)
	scopeLog.Scope().SetDroppedAttributesCount(2)

	logRecord1 := scopeLog.LogRecords().AppendEmpty()

	logRecord1.SetTimestamp(pcommon.Timestamp(1234567890))
	logRecord1.SetObservedTimestamp(pcommon.Timestamp(1234567890))
	logRecord1.SetSeverityNumber(plog.SeverityNumberInfo)
	logRecord1.SetSeverityText("INFO")
	logRecord1.Body().SetStr("Test log message")
	logRecord1.Attributes().PutStr("key1", "value1")
	logRecord1.Attributes().PutInt("key2", 123)
	logRecord1.Attributes().PutBool("error", true)
	logRecord1.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}))
	logRecord1.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	logRecord1.SetDroppedAttributesCount(2)
	logRecord1.SetFlags(1)

	logRecord2 := scopeLog.LogRecords().AppendEmpty()

	logRecord2.SetTimestamp(pcommon.Timestamp(123456347890))
	logRecord2.SetObservedTimestamp(pcommon.Timestamp(12345678922))
	logRecord2.SetSeverityNumber(plog.SeverityNumberDebug)
	logRecord2.SetSeverityText("DEBUG")
	logRecord2.Body().SetStr("Test log message is good")
	logRecord2.Attributes().PutStr("keyOld", "value1")
	logRecord2.Attributes().PutInt("keyNew", 123)
	logRecord2.Attributes().PutBool("error", false)
	logRecord2.SetTraceID(pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 3, 9, 10, 11, 1, 13, 14, 15, 16}))
	logRecord2.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 9}))
	logRecord2.SetDroppedAttributesCount(3)
	logRecord2.SetFlags(2)

	estimatedSize := estimateSizes(logs)

	assert.NotNil(t, estimatedSize)
	assert.Equal(t, 1, len(estimatedSize.ResourceLogSize))
	assert.Equal(t, 1, len(estimatedSize.ScopeSize[0]))
	assert.Equal(t, 1, len(estimatedSize.ScopeLogSize[0]))
	assert.Equal(t, 2, len(estimatedSize.LogRecordSize[0][0]))

	assert.Equal(t, 725, estimatedSize.ResourceLogSize[0])
	assert.Equal(t, 98, estimatedSize.ScopeSize[0][0])
	assert.Equal(t, 673, estimatedSize.ScopeLogSize[0][0])
	assert.Equal(t, 276, estimatedSize.LogRecordSize[0][0][0])
	assert.Equal(t, 289, estimatedSize.LogRecordSize[0][0][1])
}

func TestEstimateAnyValueTypeSizeBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        pcommon.Value
		expectedSize int
		description  string
	}{
		{
			name:         "StringValue",
			value:        pcommon.NewValueStr("test"),
			expectedSize: StringValueFieldSizeBytes + len("test"),
			description:  "Tests size estimation for string values",
		},
		{
			name:         "IntValue",
			value:        pcommon.NewValueInt(42),
			expectedSize: IntValueFieldSizeBytes + IntegerSizeBytes,
			description:  "Tests size estimation for integer values",
		},
		{
			name:         "DoubleValue",
			value:        pcommon.NewValueDouble(3.14),
			expectedSize: DoubleValueFieldSizeBytes + DoubleSizeBytes,
			description:  "Tests size estimation for double values",
		},
		{
			name:         "BoolValue",
			value:        pcommon.NewValueBool(true),
			expectedSize: BoolValueFieldSizeBytes + BooleanSizeBytes,
			description:  "Tests size estimation for boolean values",
		},
		{
			name: "BytesValue",
			value: func() pcommon.Value {
				val := pcommon.NewValueBytes()
				val.Bytes().FromRaw([]byte{0x01, 0x02, 0x03})
				return val
			}(),
			expectedSize: BytesValueFieldSizeBytes + 3,
			description:  "Tests size estimation for byte array values",
		},
		{
			name: "SliceValue",
			value: func() pcommon.Value {
				slice := pcommon.NewValueSlice()
				slice.Slice().AppendEmpty().SetInt(1)
				slice.Slice().AppendEmpty().SetInt(2)
				return slice
			}(),
			expectedSize: ArrayValueFieldSizeBytes + ValuesFieldSizeBytes +
				(IntValueFieldSizeBytes+IntegerSizeBytes)*2,
			description: "Tests size estimation for slice values",
		},
		{
			name: "MapValue",
			value: func() pcommon.Value {
				m := pcommon.NewValueMap()
				m.Map().PutStr("key1", "value1")
				m.Map().PutInt("key2", 100)
				return m
			}(),
			expectedSize: KvListValueFieldSizeBytes + ValuesFieldSizeBytes +
				(KeyFieldSizeBytes + len("key1")) + (ValueFieldSizeBytes + StringValueFieldSizeBytes + len("value1")) +
				(KeyFieldSizeBytes + len("key2")) + (ValueFieldSizeBytes + IntegerSizeBytes + IntValueFieldSizeBytes),
			description: "Tests size estimation for map values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSize := estimateAnyValueTypeSizeBytes(tt.value)
			assert.Equal(t, tt.expectedSize, actualSize)
		})
	}
}
