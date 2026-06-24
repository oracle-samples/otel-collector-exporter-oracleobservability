// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/loganalytics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.uber.org/zap"
)

// MockLogAnalyticsClient is a mock implementation of the LogAnalyticsClient interface
type MockLogAnalyticsClient struct {
	mock.Mock
}

// MockUploadOtlpLogsResponse is a mock implementation of the UploadOtlpLogsResponse interface
type MockUploadOtlpLogsResponse struct {
	loganalytics.UploadOtlpLogsResponse
	RawResponse *http.Response
}

// HTTPResponse returns the HTTP response from the mock
func (m MockUploadOtlpLogsResponse) HTTPResponse() *http.Response {
	return m.RawResponse
}

func (m *MockLogAnalyticsClient) UploadOtlpLogs(ctx context.Context, request loganalytics.UploadOtlpLogsRequest) (loganalytics.UploadOtlpLogsResponse, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return loganalytics.UploadOtlpLogsResponse{
			RawResponse: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
			},
		}, args.Error(1)
	}
	mockResp := args.Get(0).(MockUploadOtlpLogsResponse)
	return mockResp.UploadOtlpLogsResponse, args.Error(1)
}

func TestGenerateRetryToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "Empty data",
			data:     []byte{},
			expected: "cbf29ce484222325",
		},
		{
			name:     "Simple string",
			data:     []byte("test"),
			expected: "f9e6e6ef197c2b25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateRetryToken(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSendData(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	config := &Config{
		NamespaceName: "test-namespace",
		LogGroupID:    "test-group-id",
	}

	tests := []struct {
		name           string
		data           []byte
		mockResponse   MockUploadOtlpLogsResponse
		mockError      error
		expectedError  bool
		permanentError bool
		expectedErrMsg string
		expectedStatus int
	}{
		{
			name: "Successful upload",
			data: []byte(`{"test": "data"}`),
			mockResponse: MockUploadOtlpLogsResponse{
				UploadOtlpLogsResponse: loganalytics.UploadOtlpLogsResponse{
					RawResponse: &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
					},
				},
			},
			mockError:      nil,
			expectedError:  false,
			expectedStatus: http.StatusOK,
		},
		{
			name: "Successful upload with 204",
			data: []byte(`{"test": "data"}`),
			mockResponse: MockUploadOtlpLogsResponse{
				UploadOtlpLogsResponse: loganalytics.UploadOtlpLogsResponse{
					RawResponse: &http.Response{
						StatusCode: http.StatusNoContent,
						Status:     "204 No Content",
					},
				},
			},
			mockError:      nil,
			expectedError:  false,
			expectedStatus: http.StatusNoContent,
		},
		{
			name: "SDK error with success response is returned",
			data: []byte(`{"test": "data"}`),
			mockResponse: MockUploadOtlpLogsResponse{
				UploadOtlpLogsResponse: loganalytics.UploadOtlpLogsResponse{
					RawResponse: &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
					},
				},
			},
			mockError:      errors.New("sdk error"),
			expectedError:  true,
			expectedErrMsg: "sdk error",
			expectedStatus: http.StatusOK,
		},
		{
			name: "Nil response returns SDK error",
			data: []byte(`{"test": "data"}`),
			mockResponse: MockUploadOtlpLogsResponse{
				UploadOtlpLogsResponse: loganalytics.UploadOtlpLogsResponse{},
			},
			mockError:      errors.New("sdk error"),
			expectedError:  true,
			expectedErrMsg: "sdk error",
		},
		{
			name: "Bad request error",
			data: []byte(`{"test": "data"}`),
			mockResponse: MockUploadOtlpLogsResponse{
				UploadOtlpLogsResponse: loganalytics.UploadOtlpLogsResponse{
					RawResponse: &http.Response{
						StatusCode: http.StatusBadRequest,
						Status:     "400 Bad Request",
					},
				},
			},
			mockError:      nil,
			expectedError:  true,
			permanentError: true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Client error",
			data: []byte(`{"test": "data"}`),
			mockResponse: MockUploadOtlpLogsResponse{
				UploadOtlpLogsResponse: loganalytics.UploadOtlpLogsResponse{
					RawResponse: &http.Response{
						StatusCode: http.StatusInternalServerError,
						Status:     "500 Internal Server Error",
					},
				},
			},
			mockError:      errors.New("client error"),
			expectedError:  true,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockLogAnalyticsClient)
			worker := &defaultWorker{
				logAnalyticsClient: mockClient,
				logger:             logger,
				config:             config,
				ctx:                context.Background(),
			}

			// Set up mock expectations
			mockClient.On("UploadOtlpLogs", mock.Anything, mock.Anything).Return(tt.mockResponse, tt.mockError)

			err := worker.sendData(context.Background(), tt.data)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Equal(t, tt.permanentError, consumererror.IsPermanent(err))
				if tt.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestHandleHTTPCode(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	worker := &defaultWorker{
		logger: logger,
	}

	tests := []struct {
		name           string
		statusCode     int
		expectedError  bool
		permanentError bool
		expectedStatus string
	}{
		{
			name:           "Success status code",
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedStatus: "200 OK",
		},
		{
			name:           "No content success status code",
			statusCode:     http.StatusNoContent,
			expectedError:  false,
			expectedStatus: "204 No Content",
		},
		{
			name:           "Bad request status code",
			statusCode:     http.StatusBadRequest,
			expectedError:  true,
			permanentError: true,
			expectedStatus: "400 Bad Request",
		},
		{
			name:           "Unauthorized status code",
			statusCode:     http.StatusUnauthorized,
			expectedError:  true,
			expectedStatus: "401 Unauthorized",
		},
		{
			name:           "Not found status code",
			statusCode:     http.StatusNotFound,
			expectedError:  true,
			expectedStatus: "404 Not Found",
		},
		{
			name:           "Conflict status code",
			statusCode:     http.StatusConflict,
			expectedError:  true,
			permanentError: true,
			expectedStatus: "409 Conflict",
		},
		{
			name:           "Too many requests status code",
			statusCode:     http.StatusTooManyRequests,
			expectedError:  true,
			expectedStatus: "429 Too Many Requests",
		},
		{
			name:           "Internal server error status code",
			statusCode:     http.StatusInternalServerError,
			expectedError:  true,
			expectedStatus: "500 Internal Server Error",
		},
		{
			name:           "Unhandled status code",
			statusCode:     418,
			expectedError:  true,
			expectedStatus: "418",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode: tt.statusCode,
				Status:     tt.expectedStatus,
			}

			err := worker.handleHTTPCode(response)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Equal(t, tt.permanentError, consumererror.IsPermanent(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
