// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/loganalytics"
)

func usesFileResourcePrincipal() bool {
	return os.Getenv(auth.ResourcePrincipalVersionEnvVar) == "2.2" &&
		filepath.IsAbs(os.Getenv(auth.ResourcePrincipalRPSTEnvVar)) &&
		filepath.IsAbs(os.Getenv(auth.ResourcePrincipalPrivatePEMEnvVar))
}

type resourcePrincipalClientGeneration struct {
	client MinimalLogAnalyticsClient
}

// Each upload holds an immutable client generation. A late 401 from an older
// upload must not invalidate credentials already refreshed by another upload.
type refreshingResourcePrincipalClient struct {
	mu      sync.Mutex
	current *resourcePrincipalClientGeneration
	reload  func() (MinimalLogAnalyticsClient, error)
}

func newRefreshingResourcePrincipalClient(template loganalytics.LogAnalyticsClient) *refreshingResourcePrincipalClient {
	return &refreshingResourcePrincipalClient{
		current: &resourcePrincipalClientGeneration{client: template},
		reload: func() (MinimalLogAnalyticsClient, error) {
			provider, err := auth.ResourcePrincipalConfigurationProvider()
			if err != nil {
				return nil, err
			}
			if _, err := provider.PrivateRSAKey(); err != nil {
				return nil, err
			}
			client := template
			client.Signer = common.DefaultRequestSigner(provider)
			return client, nil
		},
	}
}

func (c *refreshingResourcePrincipalClient) generation(ctx context.Context) (*resourcePrincipalClientGeneration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.current == nil {
		client, err := c.reload()
		if err != nil {
			return nil, fmt.Errorf("failed to reload resource principal credentials: %w", err)
		}
		c.current = &resourcePrincipalClientGeneration{client: client}
	}
	return c.current, nil
}

func (c *refreshingResourcePrincipalClient) UploadOtlpLogs(ctx context.Context, request loganalytics.UploadOtlpLogsRequest) (loganalytics.UploadOtlpLogsResponse, error) {
	generation, err := c.generation(ctx)
	if err != nil {
		return loganalytics.UploadOtlpLogsResponse{}, err
	}
	response, err := generation.client.UploadOtlpLogs(ctx, request)
	unauthorized := response.RawResponse != nil && response.RawResponse.StatusCode == http.StatusUnauthorized
	if serviceErr, ok := common.IsServiceError(err); ok && serviceErr.GetHTTPStatusCode() == http.StatusUnauthorized {
		unauthorized = true
	}
	if unauthorized {
		// The SDK can cache a new token with an old key until token expiry. Force
		// the next queued upload/retry to reload, without replaying its body here.
		c.mu.Lock()
		if c.current == generation {
			c.current = nil
		}
		c.mu.Unlock()
	}
	return response, err
}
