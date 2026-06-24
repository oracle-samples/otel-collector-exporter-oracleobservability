// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"context"
	"fmt"
	"net/http"

	oci_common "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/loganalytics"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

const (
	MaxContentLengthLogsInBytes = 10485760 // 10 MB
)

const truncationSuffix = " TRUNCATED_AT_COLLECTOR"

func maxChunkPayloadSizeWithBuffer() int {
	return MaxContentLengthLogsInBytes - int(float64(MaxContentLengthLogsInBytes)*0.25)
}

type oracleobservabilityLogsExporter struct {
	cancel                    context.CancelFunc
	logger                    *zap.Logger
	config                    *Config
	settings                  component.TelemetrySettings
	oracleobservabilityWorker oracleobservabilityWorker
	ociClient                 loganalytics.LogAnalyticsClient
}

var newLogAnalyticsClientFactory = initializeOciLogAnalyticsClient
var newOracleObservabilityWorker = defaultOracleObservabilityWorker

func defaultOracleObservabilityWorker(
	ctx context.Context,
	logger *zap.Logger,
	cfg *Config,
	ociClient loganalytics.LogAnalyticsClient,
) oracleobservabilityWorker {
	return &defaultWorker{
		ctx:                ctx,
		logger:             logger,
		config:             cfg,
		logAnalyticsClient: ociClient,
	}
}

func shouldRetryOnNon2xxResponse(r oci_common.OCIOperationResponse) bool {
	if r.Error != nil {
		return true
	}

	if r.Response == nil || r.Response.HTTPResponse() == nil {
		return true
	}

	statusCode := r.Response.HTTPResponse().StatusCode
	return statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices
}

// newLogsExporter creates a new instance of the Oracle Observability logs exporter.
func newLogsExporter(ctx context.Context, params exporter.Settings, cfg component.Config) (exporter.Logs, error) {

	// Initialize zap logger
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize zap logger: %v", err)
	}
	logger = logger.Named("oracleobservability")

	// Type assert the provided configuration to the expected *Config type.
	config, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid configuration type, expected *Config but got %T", cfg)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Initialize OCI client
	logAnalyticsClient, err := newLogAnalyticsClientFactory(config.AuthType, config.OciConfiguration,
		string(config.ConfigFilePath), string(config.ConfigProfile), string(config.PrivateKeyPassphrase))
	if err != nil {
		return nil, err
	}
	logger.Info("OCI Log Analytics client initialized successfully")

	customRetryPolicy := oci_common.NewRetryPolicyWithOptions(
		oci_common.ReplaceWithValuesFromRetryPolicy(oci_common.DefaultRetryPolicyWithoutEventualConsistency()),
		oci_common.WithEventualConsistency(),
		oci_common.WithShouldRetryOperation(shouldRetryOnNon2xxResponse),
	)

	logAnalyticsClient.Configuration.RetryPolicy = &customRetryPolicy
	logAnalyticsClient.BaseClient.Interceptor = customRequestInterceptor

	exp := &oracleobservabilityLogsExporter{
		logger:    logger,
		config:    config,
		ociClient: logAnalyticsClient,
		settings:  params.TelemetrySettings,
	}

	// Wrap the exporter with helper functions for queuing, retry, etc.
	return exporterhelper.NewLogs(
		ctx,
		params,
		cfg,
		exp.pushLogData,
		// Exporter just forwards or uploads data without any alterations.
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		exporterhelper.WithTimeout(config.TimeoutConfig),
		exporterhelper.WithQueue(config.QueueConfig),
		exporterhelper.WithRetry(config.BackOffConfig),
		exporterhelper.WithStart(exp.Start),
		exporterhelper.WithShutdown(exp.Shutdown),
	)
}

func customRequestInterceptor(req *http.Request) error {
	req.Header.Set("content-encoding", "gzip")
	return nil
}

// getConfigProvider determines and creates the appropriate OCI configuration provider based on the AuthType.
// If the config_file_path is not specified in the config, it uses the default OCI configuration provider.
// If the config_profile is not specified, it uses the default profile from the specified config file.
// If both config_file_path and config_profile are specified, it uses the specified profile from the config file.
func getConfigFileProvider(ociConfigFile OciConfig, configFilePath string, configProfile string, privateKeyPassphrase string) (oci_common.ConfigurationProvider, error) {

	if isOciConfigUsed(ociConfigFile) {
		return oci_common.NewRawConfigurationProvider(string(ociConfigFile.Tenancy),
			string(ociConfigFile.User),
			string(ociConfigFile.Region),
			string(ociConfigFile.FingerPrint),
			string(ociConfigFile.PrivateKey),
			&privateKeyPassphrase,
		), nil
	}

	if configFilePath == "" {
		return oci_common.DefaultConfigProvider(), nil
	}

	if configProfile == "" {
		configProfile = "DEFAULT"
	}

	return oci_common.ConfigurationProviderFromFileWithProfile(
		configFilePath,
		configProfile,
		privateKeyPassphrase,
	)
}

func initializeOciLogAnalyticsClient(authType AuthenticationType, ociConfiguration OciConfig,
	configFilePath string, configProfile string, privateKeyPassphrase string) (loganalytics.LogAnalyticsClient, error) {

	var configProvider oci_common.ConfigurationProvider
	var err error

	switch authType {
	case InstancePrincipal:
		configProvider, err = auth.InstancePrincipalConfigurationProvider()

	case ConfigFile:
		configProvider, err = getConfigFileProvider(ociConfiguration, configFilePath, configProfile, privateKeyPassphrase)

	default:
		return loganalytics.LogAnalyticsClient{}, fmt.Errorf("unsupported auth_type %q", authType)
	}

	if err != nil {
		return loganalytics.LogAnalyticsClient{}, err
	}

	return loganalytics.NewLogAnalyticsClientWithConfigurationProvider(configProvider)
}

// pushLogData processes and exports log data (plog.Logs) either as a single batch or in smaller chunks.
func (oe *oracleobservabilityLogsExporter) pushLogData(ctx context.Context, logs plog.Logs) error {

	oe.logger.Info("received log data")

	if logs.ResourceLogs().Len() == 0 {
		oe.logger.Error("received empty log, skipping processing")
		return nil
	}

	jsonMarshaler := plog.JSONMarshaler{}

	plogsJSON, err := jsonMarshaler.MarshalLogs(logs)
	if err != nil {
		return consumererror.NewPermanent(fmt.Errorf("failed to marshal logs: %v", err))
	}

	maxPayloadSizeWithBuffer := maxChunkPayloadSizeWithBuffer()
	plogsJSONSize := len(plogsJSON)

	if plogsJSONSize < maxPayloadSizeWithBuffer {
		oe.logger.Info("sending logs as single chunk to OCI Log Analytics")
		return oe.sendPayload(ctx, plogsJSON)
	}

	return oe.chunkPlogs(ctx, logs, jsonMarshaler)
}

func (oe *oracleobservabilityLogsExporter) sendPayload(ctx context.Context, payload []byte) error {
	if oe.oracleobservabilityWorker == nil {
		return fmt.Errorf("oracleobservability worker is not initialized")
	}
	return oe.oracleobservabilityWorker.sendData(ctx, payload)
}

func (oe *oracleobservabilityLogsExporter) chunkPlogs(
	ctx context.Context,
	logs plog.Logs,
	jsonMarshaller plog.JSONMarshaler,
) error {

	oe.logger.Info("chunking log data to fit within the maximum allowed size limit")

	resourceLogsPointer := 0
	scopeLogsPointer := 0
	logRecordsPointer := 0

	estimatedSizes := estimateSizes(logs)

	for resourceLogsPointer < logs.ResourceLogs().Len() {
		newPlog := plog.NewLogs()

		scopeLogsCounter := 0
		resourceLogsCounter := 0
		logRecordCounter := 0

		availableBufferBytes := maxChunkPayloadSizeWithBuffer()

		if scopeLogsPointer == 0 && logRecordsPointer == 0 {
			resourceLogsCounter = calculateResourceLogsCount(estimatedSizes.ResourceLogSize, resourceLogsPointer, &availableBufferBytes)
		}

		if resourceLogsCounter != 0 {
			oe.logger.Debug("chunking logs at resource level")
			for i := resourceLogsPointer; i < resourceLogsPointer+resourceLogsCounter; i++ {
				newResourceLog := newPlog.ResourceLogs().AppendEmpty()
				currentResourceLog := logs.ResourceLogs().At(i)
				currentResourceLog.CopyTo(newResourceLog)
			}

			resourceLogsPointer += resourceLogsCounter

			if err := oe.sendChunk(ctx, newPlog, jsonMarshaller); err != nil {
				return err
			}

			continue

		} else {
			newResourceLog := newPlog.ResourceLogs().AppendEmpty()
			currentResource := logs.ResourceLogs().At(resourceLogsPointer).Resource()
			newResource := newResourceLog.Resource()
			currentResource.CopyTo(newResource)

			availableBufferBytes -= estimatedSizes.ResourceAttributesSize[resourceLogsPointer]

			if availableBufferBytes < 0 {
				oe.logger.Error("dropping log record: unable to split or truncate further")
				resourceLogsPointer++
				continue
			}

			if logRecordsPointer == 0 {
				scopeLogsCounter = calculateScopeLogsCount(estimatedSizes.ScopeLogSize, scopeLogsPointer, resourceLogsPointer,
					&availableBufferBytes)
			}

			currentResourceLog := logs.ResourceLogs().At(resourceLogsPointer)

			if scopeLogsCounter != 0 {
				oe.logger.Debug("chunking logs at scope level")
				for i := scopeLogsPointer; i < scopeLogsPointer+scopeLogsCounter; i++ {
					lastResourceLog := newPlog.ResourceLogs().At(newPlog.ResourceLogs().Len() - 1)
					newScopeLog := lastResourceLog.ScopeLogs().AppendEmpty()
					currentScopeLog := currentResourceLog.ScopeLogs().At(i)
					currentScopeLog.CopyTo(newScopeLog)
				}

				scopeLogsPointer += scopeLogsCounter

				if scopeLogsPointer >= currentResourceLog.ScopeLogs().Len() {
					scopeLogsPointer = 0
					logRecordsPointer = 0
					resourceLogsPointer++
				}

				if err := oe.sendChunk(ctx, newPlog, jsonMarshaller); err != nil {
					return err
				}

				continue

			} else {
				lastResourceLog := newPlog.ResourceLogs().At(newPlog.ResourceLogs().Len() - 1)
				newScopeLog := lastResourceLog.ScopeLogs().AppendEmpty()
				currentScope := currentResourceLog.ScopeLogs().At(scopeLogsPointer).Scope()
				newScope := newScopeLog.Scope()
				currentScope.CopyTo(newScope)

				availableBufferBytes -= estimatedSizes.ScopeSize[resourceLogsPointer][scopeLogsPointer]

				currentScopeLog := currentResourceLog.ScopeLogs().At(scopeLogsPointer)

				if availableBufferBytes < 0 {
					oe.logger.Error("dropping log record: unable to split or truncate further")
					scopeLogsPointer++
					if scopeLogsPointer >= currentResourceLog.ScopeLogs().Len() {
						scopeLogsPointer = 0
						logRecordsPointer = 0
						resourceLogsPointer++
					}
					continue
				}

				logRecordCounter = calculateLogRecordsCount(estimatedSizes.LogRecordSize, logRecordsPointer, scopeLogsPointer,
					resourceLogsPointer, &availableBufferBytes)

				if logRecordCounter != 0 {
					oe.logger.Debug("chunking logs at logRecord level")
					for i := logRecordsPointer; i < logRecordsPointer+logRecordCounter; i++ {
						lastScopeLog := lastResourceLog.ScopeLogs().At(lastResourceLog.ScopeLogs().Len() - 1)
						currentLogRecord := currentScopeLog.LogRecords().At(i)
						newLogRecord := lastScopeLog.LogRecords().AppendEmpty()
						currentLogRecord.CopyTo(newLogRecord)
					}

					logRecordsPointer += logRecordCounter

					if logRecordsPointer >= currentScopeLog.LogRecords().Len() {
						scopeLogsPointer++
						logRecordsPointer = 0
						if scopeLogsPointer >= currentResourceLog.ScopeLogs().Len() {
							scopeLogsPointer = 0
							logRecordsPointer = 0
							resourceLogsPointer++
						}
					}

					if err := oe.sendChunk(ctx, newPlog, jsonMarshaller); err != nil {
						return err
					}

					continue

				} else {
					oe.logger.Debug("truncating body of logRecord")
					currentLogRecord := currentScopeLog.LogRecords().At(logRecordsPointer)
					logRecordSizeBytes := estimatedSizes.LogRecordSize[resourceLogsPointer][scopeLogsPointer][logRecordsPointer]

					logRecordSizeWithoutBodyBytes := logRecordSizeBytes - estimateAnyValueTypeSizeBytes(currentLogRecord.Body())
					availableBufferBytes -= logRecordSizeWithoutBodyBytes

					if availableBufferBytes < 0 {
						oe.logger.Error("dropping log record: unable to split or truncate further")
						logRecordsPointer++
						if logRecordsPointer >= currentScopeLog.LogRecords().Len() {
							scopeLogsPointer++
							logRecordsPointer = 0
							if scopeLogsPointer >= currentResourceLog.ScopeLogs().Len() {
								scopeLogsPointer = 0
								logRecordsPointer = 0
								resourceLogsPointer++
							}
						}
						continue
					}

					bodyString := currentLogRecord.Body().AsString()
					truncatedBodyString := ""

					if len(bodyString) > availableBufferBytes {
						if availableBufferBytes <= len(truncationSuffix) {
							truncatedBodyString = truncationSuffix
						} else {
							truncatedBodyString = bodyString[:availableBufferBytes-len(truncationSuffix)] + truncationSuffix
						}
					} else {
						truncatedBodyString = bodyString
					}

					lastScopeLog := lastResourceLog.ScopeLogs().At(lastResourceLog.ScopeLogs().Len() - 1)
					newLogRecord := lastScopeLog.LogRecords().AppendEmpty()
					currentLogRecord.CopyTo(newLogRecord)
					newLogRecord.Body().SetStr(truncatedBodyString)

					logRecordsPointer++

					if logRecordsPointer >= currentScopeLog.LogRecords().Len() {
						scopeLogsPointer++
						logRecordsPointer = 0
						if scopeLogsPointer >= currentResourceLog.ScopeLogs().Len() {
							scopeLogsPointer = 0
							logRecordsPointer = 0
							resourceLogsPointer++
						}
					}

					if err := oe.sendChunk(ctx, newPlog, jsonMarshaller); err != nil {
						return err
					}

					continue
				}
			}
		}
	}
	return nil
}

func (oe *oracleobservabilityLogsExporter) sendChunk(ctx context.Context, logs plog.Logs, marshaller plog.JSONMarshaler) error {
	jsonBytes, err := marshaller.MarshalLogs(logs)
	if err != nil {
		return consumererror.NewPermanent(fmt.Errorf("failed to marshal chunked logs: %v", err))
	}
	oe.logger.Info("sending chunked logs to OCI Log Analytics")

	return oe.sendPayload(ctx, jsonBytes)
}

// calculateResourceLogsCount determines how many resource logs can fit within the remaining buffer
func calculateResourceLogsCount(resourceLogSizes []int, resourceLogsPointer int, availableBufferBytes *int) int {

	resourceLogsCounter := 0

	// Iterate through resourceLogSizes starting at resourceLogsPointer
	for i := resourceLogsPointer; i < len(resourceLogSizes); i++ {
		// Check if the current resource log size can fit in the remaining buffer
		if resourceLogSizes[i] <= *availableBufferBytes {
			resourceLogsCounter++
			*availableBufferBytes -= resourceLogSizes[i]
		} else {
			break // Stop if the current resource log cannot fit
		}
	}

	return resourceLogsCounter
}

// calculateScopeLogsCount determines how many scope logs can fit in the remaining buffer
func calculateScopeLogsCount(scopeLogsSize [][]int, scopeLogsPointer int, resourceLogsPointer int, availableBufferBytes *int) int {

	scopeLogsCounter := 0

	// Iterate through scopeLogsSize for the given resourceLogsPointer
	for i := scopeLogsPointer; i < len(scopeLogsSize[resourceLogsPointer]); i++ {
		// Check if the current scope log size can fit in the remaining buffer
		if scopeLogsSize[resourceLogsPointer][i] <= *availableBufferBytes {
			scopeLogsCounter++
			*availableBufferBytes -= scopeLogsSize[resourceLogsPointer][i]
		} else {
			break // Stop if the current scope log cannot fit
		}
	}

	return scopeLogsCounter
}

// calculateLogRecordsCount determines how many log records can fit in the remaining buffer
func calculateLogRecordsCount(logRecordsSize [][][]int, logRecordPointer int, scopeLogsPointer int, resourceLogsPointer int, availableBufferBytes *int) int {

	logRecordCounter := 0

	// Iterate through logRecordsSize for the given resourceLogsPointer and scopeLogsPointer
	for i := logRecordPointer; i < len(logRecordsSize[resourceLogsPointer][scopeLogsPointer]); i++ {
		// Check if the current log record size can fit in the remaining buffer
		if logRecordsSize[resourceLogsPointer][scopeLogsPointer][i] <= *availableBufferBytes {
			logRecordCounter++
			*availableBufferBytes -= logRecordsSize[resourceLogsPointer][scopeLogsPointer][i] // Reduce the buffer size
		} else {
			break // Stop if the current log record cannot fit
		}
	}

	return logRecordCounter
}

func (oe *oracleobservabilityLogsExporter) Start(ctx context.Context, host component.Host) (err error) {

	oe.logger.Info("starting oracleobservability exporter ...")

	ctx, oe.cancel = context.WithCancel(ctx)

	// Initialize the oracleobservability worker
	oe.oracleobservabilityWorker = newOracleObservabilityWorker(ctx, oe.logger, oe.config, oe.ociClient)

	return nil
}

func (oe *oracleobservabilityLogsExporter) Shutdown(ctx context.Context) (err error) {

	oe.logger.Info("shutting down oracleobservability exporter")

	// Cancel context if applicable
	if oe.cancel != nil {
		oe.cancel()
		oe.cancel = nil
	}

	if oe.oracleobservabilityWorker != nil {
		oe.oracleobservabilityWorker = nil
	}

	return nil
}
