# Oracle Observability Exporter

The `oracleobservability` exporter sends OpenTelemetry **logs** from an OpenTelemetry Collector to [OCI Log Analytics](https://docs.oracle.com/en-us/iaas/log-analytics/home.htm).

- Signal support: `logs`
- Component type: `oracleobservability`
- Stability: `stable`
- Go import path: `github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter`

## What This Exporter Does

- Exports OTLP logs to OCI Log Analytics using the [`UploadOtlpLogs` API](https://docs.oracle.com/en-us/iaas/log-analytics/doc/upload-opentelemetry-logs.html).
- Supports OCI authentication using:
  - `config_file`
  - `instance_principal`
- Supports OCI config in two ways when using `config_file`:
  - `oci_config_file_path` (+ optional `config_profile`)
  - inline `oci_config` object
- Supports standard Collector exporter settings such as `timeout`, `sending_queue`, and `retry_on_failure`.

## Configuration

### Required Fields

- `namespace`: OCI Log Analytics namespace.
- `log_group_id`: OCI Log Group OCID (used for authorization and routing).
- `auth_type`: `config_file` or `instance_principal`.

### Required Collector Extension

The startup configuration must define the `file_storage` extension and include it in `service.extensions`. The recommended exporter queue configuration sets `sending_queue.storage: file_storage`; the collector will fail to start if that storage extension is referenced but not configured.

### Authentication Fields

#### 1. `auth_type: config_file`

Use one of the following:

- OCI config file:
  - `oci_config_file_path` (optional; defaults to OCI SDK default lookup)
  - `config_profile` (optional; default `DEFAULT`)
  - `private_key_passphrase` (optional)

- Inline OCI config (`oci_config`):
  - `fingerprint`
  - `private_key`
  - `tenancy`
  - `region`
  - `user`

If both `oci_config_file_path` and `oci_config` are set, `oci_config` is used.

#### 2. `auth_type: instance_principal`

- Runs with OCI instance principal credentials.
- Do **not** set `oci_config` with this mode.

### Exporter Helper Fields

Supported under this exporter:

- `timeout`
- `sending_queue`
- `retry_on_failure`

Default settings:

- `auth_type: config_file`
- `sending_queue.enabled: true`
- `sending_queue.num_consumers: 10`
- `sending_queue.queue_size: 1000`
- `sending_queue.block_on_overflow: true`
- `sending_queue.storage: <unset>` (in-memory queue by exporter default)
- `retry_on_failure.enabled: true`
- `retry_on_failure.initial_interval: 5s`
- `retry_on_failure.max_interval: 30s`
- `retry_on_failure.max_elapsed_time: 0` (unlimited retries)
- `timeout: 0` (disabled)

## Example Collector Config

### A) Config file authentication

```yaml
exporters:
  oracleobservability:
    auth_type: config_file
    namespace: "<oci-loganalytics-namespace>"
    log_group_id: "ocid1.loganalyticsloggroup.oc1..<unique_id>"
    oci_config_file_path: "/etc/otel/oci/config"
    config_profile: "DEFAULT"
    private_key_passphrase: "<optional-passphrase>"

    timeout: 10s
    sending_queue:
      enabled: true
      num_consumers: 10
      queue_size: 1000
      block_on_overflow: true
      storage: file_storage
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 0s

extensions:
  file_storage:
    directory: /tmp/otel/file_storage
    create_directory: true

service:
  extensions: [file_storage]
  pipelines:
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [oracleobservability]
```

### B) Inline OCI config authentication

```yaml
exporters:
  oracleobservability:
    auth_type: config_file
    namespace: "<oci-loganalytics-namespace>"
    log_group_id: "ocid1.loganalyticsloggroup.oc1..<unique_id>"
    oci_config:
      fingerprint: "<fingerprint>"
      private_key: |
        -----BEGIN PRIVATE KEY-----
        <key-content>
        -----END PRIVATE KEY-----
      tenancy: "ocid1.tenancy.oc1..<unique_id>"
      region: "us-phoenix-1"
      user: "ocid1.user.oc1..<unique_id>"
```

### C) Instance principal authentication

```yaml
exporters:
  oracleobservability:
    auth_type: instance_principal
    namespace: "<oci-loganalytics-namespace>"
    log_group_id: "ocid1.loganalyticsloggroup.oc1..<unique_id>"
```

## Use This Exporter In a Custom Collector

Use OpenTelemetry Collector Builder (`ocb`) to build your own distribution with this exporter.

### 1. Create builder manifest (`builder-config.yaml`)

```yaml
dist:
  module: github.com/example/otelcol-custom
  name: otelcol-custom
  description: Custom collector with Oracle Observability exporter
  output_path: ./_build

exporters:
  - gomod: github.com/oracle-samples/otel-collector-exporter-oracleobservability v0.1.0
    import: github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter
    name: oracleobservabilityexporter

receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v0.153.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.153.0
  - gomod: go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.153.0

extensions:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.153.0
  - gomod: go.opentelemetry.io/collector/extension/zpagesextension v0.153.0

providers:
  - gomod: go.opentelemetry.io/collector/confmap/provider/envprovider v1.59.0
  - gomod: go.opentelemetry.io/collector/confmap/provider/fileprovider v1.59.0
  - gomod: go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.59.0
```

### 2. Build the binary

```bash
go install go.opentelemetry.io/collector/cmd/builder@v0.153.0
builder --config builder-config.yaml
```

### 3. Run your custom collector

```bash
./_build/otelcol-custom --config /path/to/collector-config.yaml
```

## Build From a Local Checkout

You can validate exporter changes from your local checkout before publishing a release tag.

### 1) Build a local custom collector with your working tree

Use `path` to point to your local checkout:

```yaml
exporters:
  - gomod: github.com/oracle-samples/otel-collector-exporter-oracleobservability v0.1.0
    import: github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter
    name: oracleobservabilityexporter
    path: /absolute/path/to/otel-collector-exporter-oracleobservability
```

Then run:

```bash
builder --config builder-config.yaml
```

This compiles a collector binary using your unmerged local code.

### 2) Run a smoke test pipeline

- Start collector with `oracleobservability` exporter configured.
- Send logs via OTLP (for example using `otel-cli`, SDK app, or another collector).
- Verify:
  - collector starts successfully
  - logs are exported to OCI Log Analytics
  - retry/queue behavior is visible under transient failures

### 3) Run local quality gates

```bash
go test ./...
go test -race ./...
go vet ./...
```

## OCI IAM Policies

The exporter calls the OCI Log Analytics `UploadOtlpLogs` API. Grant permissions to the principal used by the selected authentication mode, scoped to the compartment that contains the target Log Analytics log group.

Placeholders:

- `<log_group_compartment_ocid>`: OCID of the compartment that contains the target Log Analytics log group.
- `<otel_config_file_user_group>`: IAM group containing the user whose OCI config file or inline `oci_config` is used by the exporter.
- `<instance_ocid>`: OCID of the OCI Compute instance that runs the collector with `auth_type: instance_principal`.
- `otel-exporter-instance-ppl`: dynamic group containing OCI Compute instances that run the exporter with instance principal authentication.

### Config file based authentication (`auth_type: config_file`)

Use this when the exporter signs requests with an OCI config file or inline `oci_config`. The OCI user from that config must be a member of the IAM group in the policy.

Recommended least-privilege upload policy:

```text
allow group <otel_config_file_user_group> to {LOG_ANALYTICS_LOG_GROUP_UPLOAD_LOGS} in compartment id <log_group_compartment_ocid>
```

Equivalent broader option using the Log Analytics log group resource type:

```text
allow group <otel_config_file_user_group> to use loganalytics-log-group in compartment id <log_group_compartment_ocid>
```

Broader Log Analytics resource-family option:

```text
allow group <otel_config_file_user_group> to use loganalytics-resources-family in compartment id <log_group_compartment_ocid>
```

### Instance principal authentication (`auth_type: instance_principal`)

Use this when the exporter runs on an OCI Compute instance and signs requests as an instance principal. No local OCI config file or API key is required, but the instance must be included in a dynamic group and that dynamic group must be granted permission to upload logs.

Create a dynamic group:

1. In the OCI Console, go to **Identity & Security** > **Domains** > **Dynamic groups**.
2. Select **Create Dynamic Group**.
3. Enter a unique name, for example:

```text
otel-exporter-instance-ppl
```

4. Enter this matching rule, replacing `<instance_ocid>` with the OCID of the compute instance that will run the collector:

```text
All {instance.id = '<instance_ocid>'}
```

5. Create the dynamic group.

Recommended least-privilege upload policy:

```text
allow dynamic-group otel-exporter-instance-ppl to {LOG_ANALYTICS_LOG_GROUP_UPLOAD_LOGS} in compartment id <log_group_compartment_ocid>
```

If the exporter will run on multiple instances, prefer a tag-based or compartment-based dynamic group rule instead of adding each instance OCID manually.

### Service policy prerequisite (tenancy-level)

```text
allow service loganalytics to READ loganalytics-features-family in tenancy
```

Notes:

- Scope policies to the compartment containing the target Log Analytics log group.
- If multiple compartments are used, repeat statements per compartment or use a broader compartment/tenancy scope only if required.
- For `config_file`, the policy subject is the IAM group that contains the OCI user, not the config file itself.
- For `instance_principal`, allow time for dynamic group and policy changes to propagate before testing ingestion.
- References:
  - OCI Log Analytics OpenTelemetry upload API: https://docs.oracle.com/en-us/iaas/log-analytics/doc/upload-opentelemetry-logs.html
  - OCI Log Analytics IAM policy details: https://docs.oracle.com/en-us/iaas/log-analytics/doc/iam-policies-upload-open-telemetry-logs.html
  - Log Analytics policy overview: https://docs.oracle.com/en-us/iaas/log-analytics/doc/required-iam-policy.html
  - OCI dynamic groups: https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/managingdynamicgroups.htm
  - OCI instance principals and dynamic group policies: https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/callingservicesfrominstances.htm

## Recommendations

- Use `memory_limiter` and `batch` processors in the logs pipeline.
- Start with queue enabled and monitor backpressure.
- Use `instance_principal` when running collector inside OCI compute/container environments.
- Monitor Collector logs and OCI Log Analytics ingestion status during rollout.