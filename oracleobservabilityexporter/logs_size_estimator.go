// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

const (
	BooleanSizeBytes = 1
	DoubleSizeBytes  = 8
	IntegerSizeBytes = 8

	DroppedAttributesCountSizeBytes = 4
	FlagsSizeBytes                  = 4
	SeverityNumberSizeBytes         = 4
	SpanIDSizeBytes                 = 8
	TimestampSizeBytes              = 8
	TraceIDSizeBytes                = 16

	AttributesFieldSizeBytes             = 10
	BodyFieldSizeBytes                   = 4
	DroppedAttributesCountFieldSizeBytes = 23
	FlagsFieldSizeBytes                  = 5
	KeyFieldSizeBytes                    = 3
	LogRecordFieldSize                   = 10
	NameFieldSizeBytes                   = 4
	ObservedTimeUnixNanoFieldSizeBytes   = 20
	ResourceFieldSize                    = 8
	ScopeFieldSizeBytes                  = 5
	ScopeLogsFieldSize                   = 9
	SeverityNumberFieldSizeBytes         = 14
	SeverityTextFieldSizeBytes           = 12
	SpanIdFieldSizeBytes                 = 6
	TimeUnixNanoFieldSizeBytes           = 12
	TraceIdFieldSizeBytes                = 7
	ValueFieldSizeBytes                  = 5
	ValuesFieldSizeBytes                 = 6
	VersionFieldSizeBytes                = 7

	ArrayValueFieldSizeBytes  = 10
	BoolValueFieldSizeBytes   = 9
	BytesValueFieldSizeBytes  = 10
	DoubleValueFieldSizeBytes = 11
	EmptyValueFieldSizeBytes  = 10
	IntValueFieldSizeBytes    = 8
	KvListValueFieldSizeBytes = 11
	StringValueFieldSizeBytes = 11
)

type OTLPLogSize struct {
	ResourceLogSize        []int
	ScopeSize              [][]int
	ScopeLogSize           [][]int
	LogRecordSize          [][][]int
	ResourceAttributesSize []int
}

func estimateSizes(logs plog.Logs) *OTLPLogSize {

	resourceLogsCount := logs.ResourceLogs().Len()

	estimatedSize := &OTLPLogSize{
		ResourceLogSize:        make([]int, resourceLogsCount),
		ScopeSize:              make([][]int, resourceLogsCount),
		ScopeLogSize:           make([][]int, resourceLogsCount),
		LogRecordSize:          make([][][]int, resourceLogsCount),
		ResourceAttributesSize: make([]int, resourceLogsCount),
	}

	scopeLogsLen := 0
	logRecordsLen := 0

	for i := 0; i < resourceLogsCount; i++ {
		resourceLogSize := 0
		scopeLogsSize := ScopeFieldSizeBytes

		resourceLog := logs.ResourceLogs().At(i)

		resourceAttributesSize := estimateMapSizeBytes(resourceLog.Resource().Attributes())

		estimatedSize.ResourceAttributesSize[i] = resourceAttributesSize + AttributesFieldSizeBytes
		resourceSize := resourceAttributesSize + AttributesFieldSizeBytes + ResourceFieldSize

		scopeLogsLen = resourceLog.ScopeLogs().Len()
		estimatedSize.ScopeSize[i] = make([]int, scopeLogsLen)
		estimatedSize.ScopeLogSize[i] = make([]int, scopeLogsLen)
		estimatedSize.LogRecordSize[i] = make([][]int, scopeLogsLen)

		for j := 0; j < scopeLogsLen; j++ {
			scopeLog := resourceLog.ScopeLogs().At(j)
			logRecordsLen = scopeLog.LogRecords().Len()
			logRecordsSize := LogRecordFieldSize

			scopeSize := estimateScopeSizeBytes(scopeLog.Scope())
			estimatedSize.ScopeSize[i][j] = scopeSize
			estimatedSize.LogRecordSize[i][j] = make([]int, logRecordsLen)

			for k := 0; k < logRecordsLen; k++ {
				logRecordSize := estimateLogRecordSizeBytes(scopeLog.LogRecords().At(k))
				estimatedSize.LogRecordSize[i][j][k] = logRecordSize
				logRecordsSize += logRecordSize
			}

			scopeLogSize := scopeSize + logRecordsSize
			estimatedSize.ScopeLogSize[i][j] = scopeLogSize
			scopeLogsSize += scopeLogSize
		}

		resourceLogSize = scopeLogsSize + resourceSize
		estimatedSize.ResourceLogSize[i] = resourceLogSize
	}

	return estimatedSize
}

func estimateLogRecordSizeBytes(logRecord plog.LogRecord) int {

	logRecordSize := 0

	if logRecord.ObservedTimestamp() != 0 {
		logRecordSize += TimestampSizeBytes + ObservedTimeUnixNanoFieldSizeBytes
	}

	if logRecord.Timestamp() != 0 {
		logRecordSize += TimestampSizeBytes + TimeUnixNanoFieldSizeBytes
	}

	if logRecord.SeverityNumber() != 0 {
		logRecordSize += SeverityNumberSizeBytes + SeverityNumberFieldSizeBytes
	}

	if text := logRecord.SeverityText(); text != "" {
		logRecordSize += len(text) + SeverityTextFieldSizeBytes
	}

	logRecordSize += BodyFieldSizeBytes
	logRecordSize += estimateAnyValueTypeSizeBytes(logRecord.Body())

	logRecordSize += TraceIdFieldSizeBytes
	if !logRecord.TraceID().IsEmpty() {
		logRecordSize += TraceIDSizeBytes
	}

	logRecordSize += SpanIdFieldSizeBytes
	if !logRecord.SpanID().IsEmpty() {
		logRecordSize += SpanIDSizeBytes
	}

	if logRecord.DroppedAttributesCount() > 0 {
		logRecordSize += DroppedAttributesCountSizeBytes + DroppedAttributesCountFieldSizeBytes
	}

	if logRecord.Flags() != 0 {
		logRecordSize += FlagsSizeBytes + FlagsFieldSizeBytes
	}

	if attrs := logRecord.Attributes(); attrs.Len() > 0 {
		logRecordSize += AttributesFieldSizeBytes + estimateMapSizeBytes(attrs)
	}

	return logRecordSize
}

func estimateScopeSizeBytes(scope pcommon.InstrumentationScope) int {

	scopeSize := ScopeFieldSizeBytes

	if name := scope.Name(); name != "" {
		scopeSize += len(name) + NameFieldSizeBytes
	}

	if version := scope.Version(); version != "" {
		scopeSize += len(version) + VersionFieldSizeBytes
	}

	if attrs := scope.Attributes(); attrs.Len() > 0 {
		scopeSize += AttributesFieldSizeBytes + estimateMapSizeBytes(attrs)
	}

	if scope.DroppedAttributesCount() > 0 {
		scopeSize += DroppedAttributesCountSizeBytes + DroppedAttributesCountFieldSizeBytes
	}

	return scopeSize
}

func estimateMapSizeBytes(keyValueMap pcommon.Map) int {

	keyValueSize := 0

	keyValueMap.Range(func(key string, value pcommon.Value) bool {
		keyValueSize += KeyFieldSizeBytes + ValueFieldSizeBytes + len(key) + estimateAnyValueTypeSizeBytes(value)
		return true
	})

	return keyValueSize
}

func estimateAnyValueTypeSizeBytes(value pcommon.Value) int {

	valueSize := 0

	switch value.Type() {
	case pcommon.ValueTypeStr:
		valueSize += StringValueFieldSizeBytes + len(value.Str())

	case pcommon.ValueTypeInt:
		valueSize += IntValueFieldSizeBytes + IntegerSizeBytes

	case pcommon.ValueTypeDouble:
		valueSize += DoubleValueFieldSizeBytes + DoubleSizeBytes

	case pcommon.ValueTypeBool:
		valueSize += BoolValueFieldSizeBytes + BooleanSizeBytes

	case pcommon.ValueTypeBytes:
		valueSize += BytesValueFieldSizeBytes + value.Bytes().Len()

	case pcommon.ValueTypeSlice:
		valueSize += ArrayValueFieldSizeBytes + ValuesFieldSizeBytes
		for i := 0; i < value.Slice().Len(); i++ {
			valueSize += estimateAnyValueTypeSizeBytes(value.Slice().At(i))
		}

	case pcommon.ValueTypeMap:
		valueSize += KvListValueFieldSizeBytes + ValuesFieldSizeBytes + estimateMapSizeBytes(value.Map())
	}

	return valueSize
}
