// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/loganalytics"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type rpUploadFunc func(context.Context, loganalytics.UploadOtlpLogsRequest) (loganalytics.UploadOtlpLogsResponse, error)

type rpHTTPFunc func(*http.Request) (*http.Response, error)

func (f rpHTTPFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

type rpUnauthorizedError struct{}

func (rpUnauthorizedError) Error() string           { return "test unauthorized" }
func (rpUnauthorizedError) GetHTTPStatusCode() int  { return 401 }
func (rpUnauthorizedError) GetMessage() string      { return "test unauthorized" }
func (rpUnauthorizedError) GetCode() string         { return "NotAuthenticated" }
func (rpUnauthorizedError) GetOpcRequestID() string { return "test" }

func (f rpUploadFunc) UploadOtlpLogs(ctx context.Context, req loganalytics.UploadOtlpLogsRequest) (loganalytics.UploadOtlpLogsResponse, error) {
	return f(ctx, req)
}

func rpStatusClient(code int) MinimalLogAnalyticsClient {
	return rpUploadFunc(func(context.Context, loganalytics.UploadOtlpLogsRequest) (loganalytics.UploadOtlpLogsResponse, error) {
		return loganalytics.UploadOtlpLogsResponse{RawResponse: &http.Response{StatusCode: code}}, nil
	})
}

func TestResourcePrincipalReloadOnlyAfterUnauthorized(t *testing.T) {
	for _, code := range []int{200, 400, 401, 403, 429, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			reloads := 0
			c := &refreshingResourcePrincipalClient{
				current: &resourcePrincipalClientGeneration{client: rpStatusClient(code)},
				reload:  func() (MinimalLogAnalyticsClient, error) { reloads++; return rpStatusClient(200), nil },
			}
			resp, err := c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{})
			require.NoError(t, err)
			require.Equal(t, code, resp.RawResponse.StatusCode)
			require.Zero(t, reloads, "must not replay an upload internally")
			_, err = c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{})
			require.NoError(t, err)
			if code == 401 {
				require.Equal(t, 1, reloads)
			} else {
				require.Zero(t, reloads)
			}
		})
	}
}

func TestResourcePrincipalReloadFailureAndCancellation(t *testing.T) {
	want := errors.New("credential file unreadable")
	calls := 0
	c := &refreshingResourcePrincipalClient{reload: func() (MinimalLogAnalyticsClient, error) {
		calls++
		if calls == 1 {
			return nil, want
		}
		return rpStatusClient(200), nil
	}}
	_, err := c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{})
	require.ErrorIs(t, err, want)
	require.Nil(t, c.current)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.UploadOtlpLogs(ctx, loganalytics.UploadOtlpLogsRequest{})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
	_, err = c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestResourcePrincipalConcurrentLateUnauthorized(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	completed := make(chan struct{}, 2)
	calls := 0
	c := &refreshingResourcePrincipalClient{
		current: &resourcePrincipalClientGeneration{client: rpUploadFunc(func(context.Context, loganalytics.UploadOtlpLogsRequest) (loganalytics.UploadOtlpLogsResponse, error) {
			started <- struct{}{}
			<-release
			return loganalytics.UploadOtlpLogsResponse{RawResponse: &http.Response{StatusCode: 401}}, nil
		})},
		reload: func() (MinimalLogAnalyticsClient, error) { calls++; return rpStatusClient(200), nil },
	}
	for i := 0; i < 2; i++ {
		go func() {
			_, _ = c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{})
			completed <- struct{}{}
		}()
	}
	<-started
	<-started
	release <- struct{}{}
	<-completed
	_, err := c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{})
	require.NoError(t, err)
	release <- struct{}{}
	<-completed
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() { _, _ = c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{}) })
	}
	wg.Wait()
	require.Equal(t, 1, calls, "late 401 must not invalidate the new generation")
}

func TestResourcePrincipalServiceErrorAndRepeatedUnauthorized(t *testing.T) {
	calls := 0
	unauthorized := rpUploadFunc(func(context.Context, loganalytics.UploadOtlpLogsRequest) (loganalytics.UploadOtlpLogsResponse, error) {
		return loganalytics.UploadOtlpLogsResponse{}, rpUnauthorizedError{}
	})
	c := &refreshingResourcePrincipalClient{reload: func() (MinimalLogAnalyticsClient, error) { calls++; return unauthorized, nil }}
	for i := 1; i <= 3; i++ {
		_, err := c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{})
		require.Error(t, err)
		require.Equal(t, i, calls)
		require.Nil(t, c.current)
	}
}

// These unsigned synthetic tokens are only for SDK cache tests, not OCI requests.
func TestResourcePrincipalTokenFirstReloadsCorrectedKey(t *testing.T) {
	dir := t.TempDir()
	keyPath, tokenPath := filepath.Join(dir, "key"), filepath.Join(dir, "token")
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	writeKey := func(key *rsa.PrivateKey) {
		t.Helper()
		require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600))
	}
	writeToken := func(exp time.Time) {
		t.Helper()
		payload, err := json.Marshal(map[string]any{"exp": exp.Unix(), "res_tenant": "test-tenancy"})
		require.NoError(t, err)
		token := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".c3ludGhldGlj"
		require.NoError(t, os.WriteFile(tokenPath, []byte(token), 0600))
	}
	t.Setenv(auth.ResourcePrincipalVersionEnvVar, "2.2")
	t.Setenv(auth.ResourcePrincipalRegionEnvVar, "us-sanjose-1")
	t.Setenv(auth.ResourcePrincipalRPSTEnvVar, tokenPath)
	t.Setenv(auth.ResourcePrincipalPrivatePEMEnvVar, keyPath)
	t.Setenv(auth.ResourcePrincipalPrivatePEMPassphraseEnvVar, "")
	require.NoError(t, os.Unsetenv(auth.ResourcePrincipalPrivatePEMPassphraseEnvVar))
	require.True(t, usesFileResourcePrincipal())
	writeKey(oldKey)
	writeToken(time.Now().Add(-time.Hour))
	provider, err := auth.ResourcePrincipalConfigurationProvider()
	require.NoError(t, err)
	_, err = provider.KeyID()
	require.NoError(t, err)
	writeToken(time.Now().Add(time.Hour))
	_, err = provider.KeyID()
	require.NoError(t, err)
	writeKey(newKey)
	key, err := provider.PrivateRSAKey()
	require.NoError(t, err)
	require.True(t, key.PublicKey.Equal(&oldKey.PublicKey), "SDK reproduces stale-key cache")

	client, err := loganalytics.NewLogAnalyticsClientWithConfigurationProvider(provider)
	require.NoError(t, err)
	for _, mode := range []AuthenticationType{ResourcePrincipal, ConfigFile, InstancePrincipal, WorkloadIdentity} {
		worker := defaultOracleObservabilityWorker(context.Background(), zap.NewNop(), &Config{AuthType: mode}, client).(*defaultWorker)
		_, wrapped := worker.logAnalyticsClient.(*refreshingResourcePrincipalClient)
		require.Equal(t, mode == ResourcePrincipal, wrapped)
	}
	c := newRefreshingResourcePrincipalClient(client)
	c.current = &resourcePrincipalClientGeneration{client: rpStatusClient(401)}
	_, err = c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{})
	require.NoError(t, err)
	require.Nil(t, c.current)
	generation, err := c.generation(context.Background())
	require.NoError(t, err)
	reloaded := generation.client.(loganalytics.LogAnalyticsClient)
	require.Equal(t, client.Host, reloaded.Host)
	require.Equal(t, client.HTTPClient, reloaded.HTTPClient)
	// Compare deterministic PKCS#1 signatures of identical requests using the
	// reloaded SDK signer and a signer built directly from the corrected key.
	request, err := http.NewRequest(http.MethodGet, "https://example.invalid/test", nil)
	require.NoError(t, err)
	request.Header.Set("date", "Tue, 06 Oct 2026 00:00:00 GMT")
	expected := request.Clone(context.Background())
	fresh, err := auth.ResourcePrincipalConfigurationProvider()
	require.NoError(t, err)
	key, err = fresh.PrivateRSAKey()
	require.NoError(t, err)
	require.True(t, key.PublicKey.Equal(&newKey.PublicKey))
	require.NoError(t, common.DefaultRequestSigner(fresh).Sign(expected))
	require.NoError(t, reloaded.Signer.Sign(request))
	if request.Header.Get("Authorization") != expected.Header.Get("Authorization") {
		t.Fatal("reloaded request was not signed with the corrected credentials")
	}
	// Exercise the real SDK upload path and signing, without sending synthetic
	// credentials to a server. Only the restored key's signature is accepted.
	client.HTTPClient = rpHTTPFunc(func(r *http.Request) (*http.Response, error) {
		expected := r.Clone(r.Context())
		if err := common.RequestSignerExcludeBody(fresh).Sign(expected); err != nil {
			return nil, err
		}
		status := 200
		body := `{}`
		if r.Header.Get("Authorization") != expected.Header.Get("Authorization") {
			status = 401
			body = `{"code":"NotAuthenticated","message":"test signature mismatch"}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	policy := common.NoRetryPolicy()
	client.Configuration.RetryPolicy = &policy
	c = newRefreshingResourcePrincipalClient(client)
	upload := func() (loganalytics.UploadOtlpLogsResponse, error) {
		return c.UploadOtlpLogs(context.Background(), loganalytics.UploadOtlpLogsRequest{
			NamespaceName: common.String("test"), OpcMetaLoggrpid: common.String("test"),
			UploadOtlpLogsDetails: io.NopCloser(strings.NewReader(`{}`)),
		})
	}
	response, err := upload()
	require.Error(t, err)
	require.NotNil(t, response.RawResponse, "SDK request failed before dispatch: %v", err)
	require.Equal(t, 401, response.RawResponse.StatusCode)
	require.Nil(t, c.current)
	response, err = upload()
	require.NoError(t, err)
	require.Equal(t, 200, response.RawResponse.StatusCode)
	// A 401 followed by a retry deadline must still invalidate the old pair.
	retryPolicy := common.NewRetryPolicyWithOptions(
		common.ReplaceWithValuesFromRetryPolicy(common.DefaultRetryPolicyWithoutEventualConsistency()),
		common.WithShouldRetryOperation(shouldRetryOnNon2xxResponse),
	)
	client.Configuration.RetryPolicy = &retryPolicy
	c = newRefreshingResourcePrincipalClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = c.UploadOtlpLogs(ctx, loganalytics.UploadOtlpLogsRequest{
		NamespaceName: common.String("test"), OpcMetaLoggrpid: common.String("test"),
		UploadOtlpLogsDetails: io.NopCloser(strings.NewReader(`{}`)),
	})
	require.ErrorContains(t, err, "deadline")
	require.Nil(t, c.current)
	// A missing file during reload remains retryable, rather than caching failure.
	c.current = nil
	require.NoError(t, os.Remove(keyPath))
	_, err = c.generation(context.Background())
	require.Error(t, err)
	require.Nil(t, c.current)
	writeKey(newKey)
	_, err = c.generation(context.Background())
	require.NoError(t, err)
	t.Setenv(auth.ResourcePrincipalVersionEnvVar, "1.1")
	require.False(t, usesFileResourcePrincipal())
	t.Setenv(auth.ResourcePrincipalVersionEnvVar, "2.2")
	t.Setenv(auth.ResourcePrincipalRPSTEnvVar, "inline-token")
	require.False(t, usesFileResourcePrincipal())
}
