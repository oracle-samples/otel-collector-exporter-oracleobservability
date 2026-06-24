# Oracle Observability Exporter

The `oracleobservability` exporter sends OpenTelemetry **logs** from an OpenTelemetry Collector to [OCI Log Analytics](https://docs.oracle.com/en-us/iaas/log-analytics/home.htm).
Use it by building a custom OpenTelemetry Collector distribution that includes
this exporter.

- Signal support: `logs`
- Component type: `oracleobservability`
- Stability: `stable`
- Go module path: `github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter`

## Version Compatibility

Use the table below to choose the Oracle Observability Exporter version for the
OpenTelemetry Collector/Contrib version used to build your custom collector.

| Oracle Observability Exporter | OpenTelemetry Collector/Contrib |
| --- | --- |
| `v0.153.x` | `v0.153.0` |

Because this exporter is published as a Go module in the
`oracleobservabilityexporter` folder, repository tags use the submodule tag
format, for example `oracleobservabilityexporter/v0.153.0`. In an OCB manifest,
use only the module version, for example `v0.153.0`.

## Quick Start

1. Choose the exporter version from the compatibility table.
2. Add the exporter to an OpenTelemetry Collector Builder (`ocb`) manifest. See [Use This Exporter In a Custom Collector](#use-this-exporter-in-a-custom-collector).
3. Configure the Collector with:
   - `namespace`
   - `log_group_id`
   - `auth_type`
   - persistent queue storage using `file_storage`
4. Run the custom Collector with the configuration file shown in [Example Collector Config](#example-collector-config).

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

The Collector startup configuration must define the `file_storage` extension
and include it in `service.extensions`. This exporter requires `file_storage`
even when `sending_queue.storage` is not explicitly configured. The production
configuration in this README also uses `file_storage` for persistent queue
storage with `sending_queue.storage: file_storage`.

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

Exporter default settings:

- `auth_type: config_file`
- `sending_queue.enabled: true`
- `sending_queue.num_consumers: 10`
- `sending_queue.queue_size: 1000`
- `sending_queue.block_on_overflow: true`
- `sending_queue.storage`: set to `file_storage` in the production example below
- `retry_on_failure.enabled: true`
- `retry_on_failure.initial_interval: 5s`
- `retry_on_failure.max_interval: 30s`
- `retry_on_failure.max_elapsed_time: 0` (unlimited retries)
- `timeout: 0` (disabled)

## Example Collector Config

### A) Config file authentication with persistent queue

The values below for `memory_limiter`, `timeout`, `sending_queue`, and
`retry_on_failure` are sample starting points only. Tune them for your log
volume, available memory, storage performance, and operational requirements.
The `batch` processor is recommended; this example uses `timeout: 30s` and
`send_batch_size: 1024` as a starting point.

Set `file_storage.directory` to a durable, writable path for your Collector
deployment.

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 512
    spike_limit_mib: 128
  batch:
    timeout: 30s
    send_batch_size: 1024

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
    directory: "<path_to_storage>"
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

Replace only the `exporters.oracleobservability` block from the full example
above:

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

Replace only the `exporters.oracleobservability` block from the full example
above:

```yaml
exporters:
  oracleobservability:
    auth_type: instance_principal
    namespace: "<oci-loganalytics-namespace>"
    log_group_id: "ocid1.loganalyticsloggroup.oc1..<unique_id>"
```

## Advanced Log Source and Attribute Handling

OCI Log Analytics stores OpenTelemetry attributes from resource, scope, and log
record levels with each log record. By default, logs uploaded through the
[`UploadOtlpLogs` API](https://docs.oracle.com/en-us/iaas/log-analytics/doc/upload-opentelemetry-logs.html)
are processed with the Oracle-defined OpenTelemetry Logs log source.

Use the attributes below when you want to select a specific Log Analytics log
source or map OpenTelemetry attributes into Log Analytics fields. These are
OpenTelemetry attributes on the log data, not exporter configuration fields. Set
them before the exporter runs, for example with the `transform` processor.

### Override the Log Analytics log source

Set `oci_la_log_source` to route records to a specific Log Analytics log source.
This is useful when one collector pipeline reads logs from multiple files or
formats and each source needs a different parser in Log Analytics.

Set a single log source for all records in a resource:

```yaml
processors:
  transform/log_source:
    log_statements:
      - context: resource
        statements:
          - set(attributes["oci_la_log_source"], "LinuxSyslogSource")
```

Set the log source based on the file path from the `filelog` receiver:

```yaml
processors:
  transform/log_source:
    log_statements:
      - context: log
        statements:
          - set(attributes["oci_la_log_source"], "LinuxSyslogSource") where attributes["log.file.path"] == "/var/log/messages"
          - set(attributes["oci_la_log_source"], "ApacheTomcatAccessLogSource") where IsMatch(attributes["log.file.path"], ".*/access_log.*")
```

### Map attributes to Log Analytics fields

Set `oci_la_attribute_mapping` when OpenTelemetry attributes should be extracted
into specific Log Analytics fields. The value must be a JSON string containing
mapping objects.

Example mapping:

```yaml
processors:
  transform/attribute_mapping:
    log_statements:
      - context: resource
        statements:
          - set(attributes["oci_la_attribute_mapping"], "[{\"attributeName\":\"service.name\",\"laFieldName\":\"Application\"},{\"attributeName\":\"deployment.environment\",\"laFieldName\":\"Environment\"},{\"attributeName\":\"event_type\",\"laFieldName\":\"Event Type\"}]")
```

For nested map attributes, include `childAttributeName`:

```yaml
processors:
  transform/attribute_mapping:
    log_statements:
      - context: resource
        statements:
          - set(attributes["oci_la_attribute_mapping"], "[{\"attributeName\":\"system\",\"childAttributeName\":\"env\",\"laFieldName\":\"Environment\"}]")
```

For array attributes, map them to Log Analytics fields that support multi-valued data.

## Use This Exporter In a Custom Collector

Use [OpenTelemetry Collector Builder (`ocb`)](https://opentelemetry.io/docs/collector/extend/ocb/) to build your own distribution with this exporter.

### 1. Create builder manifest (`builder-config.yaml`)

```yaml
dist:
  module: github.com/example/otelcol-custom
  name: otelcol-custom
  description: Custom collector with Oracle Observability exporter
  output_path: ./_build

exporters:
  - gomod: github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter v0.153.0

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
