ORACLE AND ITS AFFILIATES DO NOT PROVIDE ANY WARRANTY WHATSOEVER, EXPRESS OR IMPLIED, FOR ANY SOFTWARE, MATERIAL OR CONTENT OF ANY KIND CONTAINED OR PRODUCED WITHIN THIS REPOSITORY, AND IN PARTICULAR SPECIFICALLY DISCLAIM ANY AND ALL IMPLIED WARRANTIES OF TITLE, NON-INFRINGEMENT, MERCHANTABILITY, AND FITNESS FOR A PARTICULAR PURPOSE. FURTHERMORE, ORACLE AND ITS AFFILIATES DO NOT REPRESENT THAT ANY CUSTOMARY SECURITY REVIEW HAS BEEN PERFORMED WITH RESPECT TO ANY SOFTWARE, MATERIAL OR CONTENT CONTAINED OR PRODUCED WITHIN THIS REPOSITORY. IN ADDITION, AND WITHOUT LIMITING THE FOREGOING, THIRD PARTIES MAY HAVE POSTED SOFTWARE, MATERIAL OR CONTENT TO THIS REPOSITORY WITHOUT ANY REVIEW. USE AT YOUR OWN RISK.

# Oracle Observability Exporter

The `oracleobservability` exporter sends OpenTelemetry **logs** from an OpenTelemetry Collector to [OCI Log Analytics](https://docs.oracle.com/en-us/iaas/log-analytics/home.htm).
Use it by building a custom OpenTelemetry Collector distribution that includes
this exporter.

- Signal support: `logs`
- Component type: `oracleobservability`
- Stability: `stable`
- Go module path: `github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter`

Exporter version `v0.162.0` supports Resource Principal authentication and is
compatible with OpenTelemetry Collector/Contrib `v0.162.0`. The examples below
use these versions.

## Contents

- [Prerequisites](#prerequisites)
- [Version Compatibility](#version-compatibility)
- [Breaking Changes and Migration](#breaking-changes-and-migration)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [What This Exporter Does](#what-this-exporter-does)
- [Configuration](#configuration)
- [Authentication Fields](#authentication-fields)
- [Example Collector Config](#example-collector-config)
- [Advanced Log Source and Attribute Handling](#advanced-log-source-and-attribute-handling)
- [Use This Exporter In a Custom Collector](#use-this-exporter-in-a-custom-collector)
- [Verify Ingestion](#verify-ingestion)
- [Troubleshooting](#troubleshooting)
- [Testing](#testing)
- [OCI IAM Policies](#oci-iam-policies)
- [Recommendations](#recommendations)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Prerequisites

To use this exporter, you need:

- Go `1.27.1` or later to build exporter and Collector version `v0.162.0`.
  Go is not needed on a host that only runs the compiled binary.
- OpenTelemetry Collector Builder (`ocb`) for your target Collector version.
- Access to OCI Log Analytics.
- An OCI Log Analytics namespace.
- An OCI Log Analytics log group OCID.
- Network access from the Collector to the OCI Log Analytics endpoint for the
  destination region over HTTPS.
- One of the following OCI authentication environments:
  - OCI configuration-file credentials.
  - An OCI Compute instance covered by an IAM dynamic group and policy for
    instance principal authentication.
  - An OCI service runtime that supplies standard OCI Resource Principal
    credentials and is covered by the required IAM policy.
  - An enhanced OKE cluster with a Kubernetes service account and IAM policy
    for OKE Workload Identity authentication.

## Version Compatibility

Use the table below to choose the Oracle Observability Exporter version for the
OpenTelemetry Collector/Contrib version used to build your custom collector.

| Oracle Observability Exporter | OpenTelemetry Collector/Contrib | Status |
| --- | --- | --- |
| `v0.162.0` | `v0.162.0` | Stable |
| `v0.156.0-dev.1` | `v0.156.0` | Development |
| `v0.155.0` | `v0.155.0` | Stable |
| `v0.153.0` | `v0.153.0` | Stable |

Because this exporter is published as a Go module in the
`oracleobservabilityexporter` folder, repository tags use the submodule tag
format, for example `oracleobservabilityexporter/v0.162.0`. In an OCB
manifest, use only the module version, for example `v0.162.0`.

## Breaking Changes and Migration

### v0.162.0: Persistent Queue Enabled by Default

This release changes the default sending queue from in-memory storage to
`file_storage`. This is a breaking configuration change: a configuration that
previously omitted queue storage can fail to start after upgrading unless the
storage extension is included in the binary, configured, and enabled.

#### Migrate to Persistent Storage

1. Include the storage extension in your OCB manifest and rebuild the Collector:

   ```yaml
   extensions:
     - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.162.0
   ```

2. Choose a writable storage directory. For containers, mount persistent storage
   at that path and ensure the Collector's runtime user can write to it. Retain
   the same storage across restarts if queued logs must survive them.
3. Merge the following into your existing Collector configuration. Replace
   `<path_to_storage>` with your chosen directory; retain your authentication,
   destination, pipeline settings, and any other enabled extensions:

   ```yaml
   extensions:
     file_storage:
       directory: <path_to_storage>

   exporters:
     oracleobservability:
       # Keep your existing authentication and destination settings.
       sending_queue:
         storage: file_storage

   service:
     extensions: [file_storage]
     # Keep your existing pipelines and other enabled extensions.
   ```

4. Before stopping the old Collector, allow pending logs to finish uploading.
   Existing in-memory records are not transferred into the new persistent queue.
5. Validate the updated configuration with the new binary, then start it and
   verify ingestion. Monitor available space on the storage filesystem:

   ```bash
   ./otelcol-dev validate --config=config.yaml
   ./otelcol-dev --config=config.yaml
   ```

Use your actual binary name and configuration path. If you already configure
persistent storage explicitly, keep its extension ID, directory, and mount;
verify that the extension is included and enabled in the upgraded Collector.

#### Run Without a Sending Queue

If you intentionally do not want queue storage, explicitly disable the sending
queue under your existing exporter configuration:

```yaml
exporters:
  oracleobservability:
    # Keep your existing authentication and destination settings.
    sending_queue:
      enabled: false
```

This removes the exporter's queue storage requirement, but also removes sending
queue buffering. It does not restore the previous in-memory queue behavior.
Do not remove a storage extension if another component still uses it.

## Quick Start

1. Choose the exporter version from the compatibility table.
2. Choose an authentication mode and grant its principal the required [OCI IAM Policies](#oci-iam-policies).
3. Build a binary that includes the exporter. See [Use This Exporter In a Custom Collector](#use-this-exporter-in-a-custom-collector).
4. Configure the Collector with:
   - `namespace`
   - `log_group_id`
   - `auth_type`
   - persistent queue storage using `file_storage`
5. Run the custom Collector with the configuration file shown in [Example Collector Config](#example-collector-config).
6. Send logs to its OTLP receiver and [verify ingestion](#verify-ingestion).

## Installation

Install this exporter by including its Go module in an OpenTelemetry Collector
Builder manifest. This repository publishes the exporter component source code;
it does not publish a pre-built Collector binary.

## What This Exporter Does

- Exports OTLP logs to OCI Log Analytics using the [`UploadOtlpLogs` API](https://docs.oracle.com/en-us/iaas/log-analytics/doc/upload-opentelemetry-logs.html).
- Supports OCI authentication using:
  - `config_file`
  - `instance_principal`
  - `workload_identity`
  - `resource_principal`
- Supports OCI config in two ways when using `config_file`:
  - `oci_config_file_path` (+ optional `config_profile`)
  - inline `oci_config` object
- Supports standard Collector exporter settings such as `timeout`, `sending_queue`, and `retry_on_failure`.

## Configuration

### Required Fields

- `namespace`: OCI Log Analytics namespace, not a Kubernetes namespace.
- `log_group_id`: OCI Log Analytics log group OCID (used for authorization and routing).

Set `auth_type` to `config_file`, `instance_principal`, `workload_identity`, or
`resource_principal`. It defaults to `config_file` when omitted.

### Required Collector Extension

The exporter enables a persistent sending queue backed by `file_storage` by
default, even when `sending_queue.storage` is omitted. Define the `file_storage`
extension and include it in `service.extensions`; otherwise, the default queue
fails to start. If you set `sending_queue.storage` to a different storage extension
ID, such as `file_storage/queue`, define that extension under `extensions` and
include the same ID in `service.extensions`. Disabling the sending queue removes
this storage requirement.

Provision writable persistent storage for the Collector and mount it at the path
configured in `file_storage.directory`. Ensure the Collector's runtime user has
read/write access and that the same storage is available after restarts. For
Kubernetes, use a suitable persistent volume and mount it into the Collector pod.
Storage provisioning, capacity, permissions, and monitoring are managed by your
deployment administrator.

Do not share the same queue files between Collector replicas; provide separate
storage for each replica. Retain the volume when replacing a pod. Do not use
`emptyDir` for a queue that must survive pod replacement.

Persistent storage can preserve queued logs across Collector restarts when the
same storage directory is retained. It requires writable disk space and adds
disk I/O; it does not guarantee zero loss or exactly-once delivery. Monitor
available disk space on the filesystem containing `file_storage.directory`.
Use your infrastructure monitoring system to alert when free space falls below
your chosen threshold, so you can expand storage or reduce incoming traffic
before writes fail. If that storage runs out of space, the Collector can fail to
save incoming logs to the queue. Do not delete queue files to free space, because
they may contain logs
that have not yet been uploaded.

Exporter retries are enabled by default for retryable upload failures to
OCI. Separately, configure the application or upstream Collector sending logs
to this Collector to retry rejected requests and requests that time out while
waiting for acknowledgement. A timeout does not necessarily mean the logs were
not accepted, so retrying can produce duplicates.

For upgrades from an in-memory default, follow
[Breaking Changes and Migration](#breaking-changes-and-migration) before starting
the upgraded Collector.

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
- Do **not** set `oci_config`, `oci_config_file_path`, `config_profile`, or
  `private_key_passphrase` with this mode.

#### 3. `auth_type: workload_identity`

- Runs with OKE Workload Identity credentials.
- Use this mode when the Collector runs in an enhanced OKE cluster, including
  OKE Virtual Nodes where instance principal authentication is not supported.
- Do **not** set `oci_config`, `oci_config_file_path`, `config_profile`, or
  `private_key_passphrase` with this mode.
- Kubernetes namespace, Kubernetes service account, and OKE cluster OCID are
  not exporter configuration fields. OCI derives those values from the running
  pod and uses them in IAM policy conditions.
- Set the OCI SDK Workload Identity environment variables on the Collector
  container:
  - `OCI_RESOURCE_PRINCIPAL_VERSION=2.2`
  - `OCI_RESOURCE_PRINCIPAL_REGION=<oci-region>`

#### 4. `auth_type: resource_principal`

- This exporter supports Resource Principal version `2.2` only; other versions
  are outside the supported and tested configuration for this authentication mode.
- Runs with OCI Resource Principal credentials supplied through the standard
  OCI SDK environment contract.
- Do **not** set `oci_config`, `oci_config_file_path`, `config_profile`, or
  `private_key_passphrase` with this mode.
- The exporter uses the OCI Go SDK Resource Principal provider. It does not
  mint credentials or implement custom request signing.

For file-backed Resource Principal credentials, the hosting runtime must supply
these environment variables to the Collector process. Use absolute file paths
inside the Collector's runtime environment:

```text
OCI_RESOURCE_PRINCIPAL_VERSION=2.2
OCI_RESOURCE_PRINCIPAL_RPST=<path_to_rpst_file>
OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM=<path_to_private_key_file>
OCI_RESOURCE_PRINCIPAL_REGION=<oci-region>
```

For an encrypted private-key file, also set
`OCI_RESOURCE_PRINCIPAL_PRIVATE_PEM_PASSPHRASE` to the absolute path of its
passphrase file. The SDK requires the key and passphrase to both be file-backed
in this mode; do not supply a literal passphrase with a key-file path. Leave
the passphrase variable unset for an unencrypted key.

Set `OCI_RESOURCE_PRINCIPAL_REGION` to a region recognized by the OCI SDK,
such as `us-sanjose-1`. The Collector rejects empty, whitespace-only, or
unrecognized values at startup.

For credential rotation without restarting the Collector:

- **Hosting OCI service or runtime:** Supplies and rotates the RPST and its
  matching private key. Provide their file paths through the environment
  variables above, and keep those paths accessible to the Collector.
- **Exporter:** Uses the OCI SDK to reload file-backed credentials when the
  cached token expires. If an upload call returns HTTP 401, the exporter attempts
  to reload file-backed credentials
  before the next upload attempt. Successful recovery requires valid, matching
  credentials and the necessary OCI permissions.
- **Collector operator:** Upload retries are enabled by default
  (`retry_on_failure.enabled: true`); no explicit setting is required unless
  retries were previously disabled. Operators can adjust the `retry_on_failure`
  settings under `exporters.oracleobservability` in the Collector configuration
  to suit their deployment. See
  [Exporter Helper Fields](#exporter-helper-fields). Retries do not rotate or
  repair credentials; the hosting service or runtime must do that.

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
- `sending_queue.storage: file_storage`
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
deployment. For containers, use a persistent volume that survives replacement
of the container or pod. Replace all angle-bracket placeholders before running.

The OTLP receiver below listens on all interfaces without TLS or client
authentication. Restrict access to trusted senders and configure transport
security for your deployment. See the
[Collector security guidance](https://opentelemetry.io/docs/security/config-best-practices/).

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
    # Only for an encrypted API signing key:
    # private_key_passphrase: "${env:OCI_API_KEY_PASSPHRASE}"

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
above. Keep the receiver, processors, extension, and service sections unchanged.
Examples B-E retain persistent queue storage and otherwise use the exporter
defaults; add timeout or retry overrides if needed.

Do not commit private keys or private-key passphrases to source control. Inject
these values through your deployment's secret-management mechanism. When the
Collector runs in a supported OCI environment, prefer instance principal,
workload identity, or resource principal authentication so that a private API
signing key is not included in the Collector configuration.

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
    sending_queue:
      storage: file_storage
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
    sending_queue:
      storage: file_storage
```

### D) OKE Workload Identity authentication

Replace only the `exporters.oracleobservability` block from the full example
above:

```yaml
exporters:
  oracleobservability:
    auth_type: workload_identity
    namespace: "<oci-loganalytics-namespace>"
    log_group_id: "ocid1.loganalyticsloggroup.oc1..<unique_id>"
    sending_queue:
      storage: file_storage
```

### E) Resource principal authentication

Replace only the `exporters.oracleobservability` block from the full example
above. The Resource Principal environment variables must be available to the
Collector process.

```yaml
exporters:
  oracleobservability:
    auth_type: resource_principal
    namespace: "<oci-loganalytics-namespace>"
    log_group_id: "ocid1.loganalyticsloggroup.oc1..<unique_id>"
    sending_queue:
      storage: file_storage
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

The minimal builder manifest below does not include `transform` or `filelog`.
Merge these entries into its existing lists (include `filelogreceiver` only
for file input), then rebuild the binary:

```yaml
processors:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.162.0
receivers:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver v0.162.0
```

In the Collector configuration, include each configured `transform/...`
processor in `service.pipelines.logs.processors` before `batch`. When using
`filelog`, configure its file paths and include it in
`service.pipelines.logs.receivers`; set `include_file_path: true` for the
file-path conditions below. See the [filelog receiver documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/receiver/filelogreceiver).

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
  - gomod: github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter v0.162.0

receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v0.162.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.162.0
  - gomod: go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.162.0

extensions:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.162.0

providers:
  - gomod: go.opentelemetry.io/collector/confmap/provider/envprovider v1.68.0
  - gomod: go.opentelemetry.io/collector/confmap/provider/fileprovider v1.68.0
  - gomod: go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.68.0
```

### 2. Build the binary

```bash
mkdir -p .bin
GOBIN="$PWD/.bin" go install go.opentelemetry.io/collector/cmd/builder@v0.162.0
./.bin/builder --config builder-config.yaml
```

### 3. Validate and run your custom collector

Use the full Collector configuration above with your credentials and deployment
values substituted. Validation can require access to the configured OCI
authentication environment; it does not prove that an upload will succeed.

```bash
./_build/otelcol-custom validate --config /path/to/collector-config.yaml
./_build/otelcol-custom --config /path/to/collector-config.yaml
```

## Verify Ingestion

1. Send a log with a unique message from an OTLP-enabled application or another
   Collector to the configured receiver (gRPC port `4317` or HTTP port `4318`).
2. Check the Collector output for upload errors. A successful receiver response
   can mean the log was queued; confirm its arrival in Log Analytics separately.
3. In OCI Log Analytics Log Explorer, select the destination region, compartment,
   log group, and a time range covering the log's timestamp. Search for the unique
   message. The default log source is **OpenTelemetry Logs** unless overridden.

Allow for batching and indexing delay. The OCI user searching the logs needs
read access; the exporter's upload permission alone does not grant that access.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Unknown exporter, receiver, processor, or extension | Include the component in the OCB manifest and rebuild the binary. Adding YAML alone does not install components. |
| Resource Principal initialization fails | Check the required environment variables, region, absolute credential paths, and file readability inside the Collector container. Do not print credential contents. |
| Upload returns HTTP 401 | Check credential validity and matching token/key files. Reloading credentials cannot repair an invalid pair. |
| Authorization failure or HTTP 404 | Check IAM policies, the principal identity, destination region, namespace, and log group OCID. A resource may be unavailable or inaccessible to the principal. |
| Queue full or storage-write errors | Check storage permissions, disk space, and upload failures. Persistent storage still needs available capacity; unlimited retries do not provide unlimited buffering. |
| Upload succeeds but logs are not visible | Check search permissions, region, log group, timestamp range, custom log-source overrides, and indexing delay. |

## Testing

Run the exporter unit tests from the repository root:

```bash
go -C oracleobservabilityexporter test ./...
```

Run additional local validation before publishing changes:

```bash
go -C oracleobservabilityexporter vet ./...
go -C oracleobservabilityexporter test -race ./...
```

The OCB manifest above imports the released module. To test unpublished local
changes, use the local module replacement and smoke test in
[CONTRIBUTING.md](CONTRIBUTING.md).

## OCI IAM Policies

The exporter calls the OCI Log Analytics `UploadOtlpLogs` API. Grant permissions to the principal used by the selected authentication mode, scoped to the compartment that contains the target Log Analytics log group.

Placeholders:

- `<log_group_compartment_ocid>`: OCID of the compartment that contains the target Log Analytics log group.
- `<otel_config_file_user_group>`: IAM group containing the user whose OCI config file or inline `oci_config` is used by the exporter.
- `<instance_ocid>`: OCID of the OCI Compute instance that runs the collector with `auth_type: instance_principal`.
- `otel-exporter-instance-ppl`: dynamic group containing OCI Compute instances that run the exporter with instance principal authentication.
- `<resource_dynamic_group>`: dynamic group containing the OCI resource whose
  hosting service supplies Resource Principal credentials to the Collector.
- `<kubernetes_namespace>`: Kubernetes namespace where the Collector pod runs.
- `<service_account_name>`: Kubernetes service account used by the Collector pod.
- `<oke_cluster_ocid>`: OCID of the enhanced OKE cluster that runs the Collector pod.

### Config file based authentication (`auth_type: config_file`)

Use this when the exporter signs requests with an OCI config file or inline `oci_config`. The OCI user from that config must be a member of the IAM group in the policy.

Recommended least-privilege upload policy:

```text
allow group <otel_config_file_user_group> to {LOG_ANALYTICS_LOG_GROUP_UPLOAD_LOGS} in compartment id <log_group_compartment_ocid>
```

Broader alternative using the Log Analytics log group resource type:

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

### OKE Workload Identity authentication (`auth_type: workload_identity`)

Use this when the exporter runs in an enhanced OKE cluster and signs requests as
the workload identity of the Collector pod. This is the recommended mode for
OKE Virtual Nodes because instance principal authentication is not supported
there.

OKE prerequisites:

- Use an enhanced OKE cluster.
- Create or choose a Kubernetes service account for the Collector.
- Run the Collector pod in the Kubernetes namespace and service account named
  in the IAM policy.
- Set `automountServiceAccountToken: true` on the pod spec.
- Set `OCI_RESOURCE_PRINCIPAL_VERSION=2.2` and
  `OCI_RESOURCE_PRINCIPAL_REGION=<oci-region>` on the Collector container.
- Do not configure `oci_config`, `oci_config_file_path`, `config_profile`, or
  `private_key_passphrase` for this exporter auth mode.

Configure the pod's `serviceAccountName`, token automount, and environment
variables in your Kubernetes workload specification, not in the
`exporters.oracleobservability` block. The OCI Go SDK uses these settings for OKE
Workload Identity authentication. For Collector storage requirements, see
[Required Collector Extension](#required-collector-extension).

Recommended least-privilege upload policy:

```text
Allow any-user to {LOG_ANALYTICS_LOG_GROUP_UPLOAD_LOGS} in compartment id <log_group_compartment_ocid> where all {
  request.principal.type = 'workload',
  request.principal.namespace = '<kubernetes_namespace>',
  request.principal.service_account = '<service_account_name>',
  request.principal.cluster_id = '<oke_cluster_ocid>'
}
```

OKE workload identities cannot be added to dynamic groups. Grant access using
an `Allow any-user` policy restricted by the workload's OKE cluster, Kubernetes
namespace, and service account, as shown above. See
[Granting Workloads Access to OCI Resources](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contenggrantingworkloadaccesstoresources.htm).

If the OKE cluster and the target Log Analytics log group are in different
compartments, configure an OKE workload mapping before using the exporter. See
[Granting Workloads Access to OCI Resources](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contenggrantingworkloadaccesstoresources.htm#Example__Using_the_Java_SDK_to_Grant_Application_Workloads_Access_to_OCI_Resources_in_a_Different_Compartment).

### Resource principal authentication (`auth_type: resource_principal`)

Use this when the hosting OCI service supplies standard Resource Principal
credentials to the Collector runtime. The OCI resource must be included in a
dynamic group and granted permission to upload to the destination Log Analytics
log group.

Recommended least-privilege upload policy:

```text
allow dynamic-group <resource_dynamic_group> to {LOG_ANALYTICS_LOG_GROUP_UPLOAD_LOGS} in compartment id <log_group_compartment_ocid>
```

The hosting service is responsible for supplying and refreshing the Resource
Principal credential material. The exporter uses the OCI SDK provider and does
not fall back to another authentication type.

### Service policy prerequisite (tenancy-level)

Ask your tenancy administrator to confirm that this service policy exists; it
may already have been created during Log Analytics onboarding. See
[Enable Access to Log Analytics and Its Resources](https://docs.oracle.com/en-us/iaas/log-analytics/doc/enable-access-logging-analytics-its-resources.html).

```text
allow service loganalytics to READ loganalytics-features-family in tenancy
```

Notes:

- Scope policies to the compartment containing the target Log Analytics log group.
- If multiple compartments are used, repeat statements per compartment or use a broader compartment/tenancy scope only if required.
- For `config_file`, the policy subject is the IAM group that contains the OCI user, not the config file itself.
- For `instance_principal`, allow time for dynamic group and policy changes to propagate before testing ingestion.
- For `workload_identity`, allow time for OKE workload identity and IAM policy changes to propagate before testing ingestion.
- For `resource_principal`, confirm that the hosting service supplies the
  required environment variables and that its resource matches the dynamic
  group before testing ingestion.
- References:
  - [OCI Log Analytics OpenTelemetry uploads and IAM policies](https://docs.oracle.com/en-us/iaas/log-analytics/doc/upload-opentelemetry-logs.html)
  - [Log Analytics prerequisite IAM policies](https://docs.oracle.com/en-us/iaas/log-analytics/doc/prerequisite-iam-policies.html)
  - [OCI dynamic groups](https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/managingdynamicgroups.htm)
  - [OCI instance principals and dynamic group policies](https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/callingservicesfrominstances.htm)
  - [OKE Workload Identity](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contenggrantingworkloadaccesstoresources.htm)
  - [OCI SDK authentication methods](https://docs.oracle.com/en-us/iaas/Content/API/Concepts/sdk_authentication_methods.htm)

## Recommendations

- Use `memory_limiter` and `batch` processors in the logs pipeline.
- Start with queue enabled and monitor backpressure.
- Use `instance_principal` when running the Collector on OCI Compute.
- Use `workload_identity` when running the Collector in OKE, especially on OKE Virtual Nodes.
- Use `resource_principal` when the hosting OCI service supplies and rotates
  standard Resource Principal credentials.
- Monitor Collector logs and OCI Log Analytics ingestion status during rollout.

## Documentation

- [OCI Log Analytics](https://docs.oracle.com/en-us/iaas/log-analytics/home.htm)
- [Upload OpenTelemetry logs to OCI Log Analytics](https://docs.oracle.com/en-us/iaas/log-analytics/doc/upload-opentelemetry-logs.html)
- [OpenTelemetry Collector Builder](https://opentelemetry.io/docs/collector/extend/ocb/)

## Contributing

This project welcomes contributions from the community. Before submitting a pull
request, please [review our contribution guide](./CONTRIBUTING.md).

## Security

Please consult the [security guide](./SECURITY.md) for our responsible security
vulnerability disclosure process.

## License

Copyright (c) 2026 Oracle and/or its affiliates.

This project is dual-licensed under the Universal Permissive License 1.0 or the
Apache License 2.0. See [LICENSE.txt](./LICENSE.txt) for details, including
warranty and limitation of liability terms.
