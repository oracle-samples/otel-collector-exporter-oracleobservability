// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"

	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.uber.org/zap"

	oci_common "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/loganalytics"
)

type oracleobservabilityWorker interface {
	sendData(context.Context, []byte) error
}

// MinimalLogAnalyticsClient is a minimal interface that only requires the methods we use
type MinimalLogAnalyticsClient interface {
	UploadOtlpLogs(ctx context.Context, request loganalytics.UploadOtlpLogsRequest) (loganalytics.UploadOtlpLogsResponse, error)
}

type defaultWorker struct {
	logAnalyticsClient MinimalLogAnalyticsClient
	logger             *zap.Logger
	config             *Config
	ctx                context.Context
}

// generateRetryToken generates a retry token based on the content of data
func generateRetryToken(data []byte) string {
	hasher := fnv.New64a()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}

// sendData sends the provided buffer data to the OCI Log Analytics using GO OCI SDK.
func (ow *defaultWorker) sendData(ctx context.Context, data []byte) error {

	// Generate a unique UUID for the OpcRetryToken
	retryToken := generateRetryToken(data)

	// Compress data
	payload, err := ow.compressData(data)
	if err != nil {
		return err
	}

	bodybuf := bytes.NewReader(payload)

	// Wrap bodybuf with io.NopCloser to satisfy the io.ReadCloser interface.
	bodyReader := io.NopCloser(bodybuf)

	uploadOtlpLogsRequest := loganalytics.UploadOtlpLogsRequest{
		NamespaceName:         oci_common.String(ow.config.NamespaceName),
		OpcMetaLoggrpid:       oci_common.String(ow.config.LogGroupID),
		UploadOtlpLogsDetails: bodyReader,
		ContentType:           oci_common.String("application/json"),
		OpcRetryToken:         oci_common.String(retryToken),
	}

	uploadOtlpLogsResponse, uploadErr := ow.logAnalyticsClient.UploadOtlpLogs(ctx, uploadOtlpLogsRequest)

	httpResponse := uploadOtlpLogsResponse.HTTPResponse()
	if httpResponse == nil {
		if uploadErr != nil {
			return fmt.Errorf("request failed: %w", uploadErr)
		}
		return fmt.Errorf("request failed as HTTP response is nil")
	}

	if err := ow.handleHTTPCode(httpResponse); err != nil {
		if uploadErr != nil {
			return fmt.Errorf("%w: %v", err, uploadErr)
		}
		return err
	}

	if uploadErr != nil {
		return fmt.Errorf("request failed despite successful HTTP status %d: %w", httpResponse.StatusCode, uploadErr)
	}

	return nil
}

func (ow *defaultWorker) compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	// First attempt
	if compressed, err := tryCompress(&buf, data); err == nil {
		return compressed, nil
	}

	// Single retry after failure
	ow.logger.Warn("compression failed, retrying once")
	buf.Reset()
	if compressed, err := tryCompress(&buf, data); err == nil {
		return compressed, nil
	} else {
		return nil, consumererror.NewPermanent(fmt.Errorf("compression failed after retry: %v", err))
	}
}

func tryCompress(buf *bytes.Buffer, data []byte) ([]byte, error) {
	gzipWriter := gzip.NewWriter(buf)
	if _, err := gzipWriter.Write(data); err != nil {
		return nil, err
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// handleHTTPCode processes an HTTP response and determines the appropriate error to return based on the status code.
func (ow *defaultWorker) handleHTTPCode(httpResponse *http.Response) error {

	headersStr := fmt.Sprintf("%v", httpResponse.Header)
	cleanedHeaders := strings.TrimPrefix(headersStr, "map[")
	cleanedHeaders = strings.TrimSuffix(cleanedHeaders, "]")

	logFields := []zap.Field{
		zap.Int("status_code", httpResponse.StatusCode),
		zap.String("status", httpResponse.Status),
		zap.String("headers", cleanedHeaders),
	}

	if httpResponse.StatusCode >= http.StatusOK && httpResponse.StatusCode < http.StatusMultipleChoices {
		ow.logger.Info("Successfully posted the log data to OCI Log Analytics", logFields...)
		return nil
	}

	ow.logger.Error("Error while sending the log data to OCI Log Analytics", logFields...)

	switch httpResponse.StatusCode {
	case http.StatusUnauthorized, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError:
		return fmt.Errorf("request failed with status %d", httpResponse.StatusCode)

	case http.StatusBadRequest, http.StatusConflict:
		return consumererror.NewPermanent(fmt.Errorf("request failed with status %d", httpResponse.StatusCode))

	default:
		return fmt.Errorf("request failed with unhandled HTTP status code %d", httpResponse.StatusCode)
	}
}

var _ oracleobservabilityWorker = &defaultWorker{}
