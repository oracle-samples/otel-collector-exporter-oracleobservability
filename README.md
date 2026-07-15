ORACLE AND ITS AFFILIATES DO NOT PROVIDE ANY WARRANTY WHATSOEVER, EXPRESS OR IMPLIED, FOR ANY SOFTWARE, MATERIAL OR CONTENT OF ANY KIND CONTAINED OR PRODUCED WITHIN THIS REPOSITORY, AND IN PARTICULAR SPECIFICALLY DISCLAIM ANY AND ALL IMPLIED WARRANTIES OF TITLE, NON-INFRINGEMENT, MERCHANTABILITY, AND FITNESS FOR A PARTICULAR PURPOSE. FURTHERMORE, ORACLE AND ITS AFFILIATES DO NOT REPRESENT THAT ANY CUSTOMARY SECURITY REVIEW HAS BEEN PERFORMED WITH RESPECT TO ANY SOFTWARE, MATERIAL OR CONTENT CONTAINED OR PRODUCED WITHIN THIS REPOSITORY. IN ADDITION, AND WITHOUT LIMITING THE FOREGOING, THIRD PARTIES MAY HAVE POSTED SOFTWARE, MATERIAL OR CONTENT TO THIS REPOSITORY WITHOUT ANY REVIEW. USE AT YOUR OWN RISK.

# Oracle Observability Exporter

The `oracleobservability` exporter sends OpenTelemetry **logs** from an OpenTelemetry Collector to [OCI Log Analytics](https://docs.oracle.com/en-us/iaas/log-analytics/home.htm).
Use it by building a custom OpenTelemetry Collector distribution that includes
this exporter.

- Signal support: `logs`
- Component type: `oracleobservability`
- Stability: `stable`
- Go module path: `github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter`

## Prerequisites

To use this exporter, you need:

- Go `1.25.12` or later for exporter and Collector version `v0.155.0`.
- OpenTelemetry Collector Builder (`ocb`) for your target Collector version.
- Access to OCI Log Analytics.
- An OCI Log Analytics namespace.
- An OCI Log Analytics log group OCID.
- One of the following OCI authentication environments:
  - OCI configuration-file credentials.
  - An OCI Compute instance covered by an IAM dynamic group and policy for
    instance principal authentication.
  - An enhanced OKE cluster with a Kubernetes service account and IAM policy
    for OKE Workload Identity authentication.

## Version Compatibility

Use the table below to choose the Oracle Observability Exporter version for the
OpenTelemetry Collector/Contrib version used to build your custom collector.

| Oracle Observability Exporter | OpenTelemetry Collector/Contrib |
| --- | --- |
| `v0.155.x` | `v0.155.0` |

Because this exporter is published as a Go module in the
`oracleobservabilityexporter` folder, repository tags use the submodule tag
format, for example `oracleobservabilityexporter/v0.155.0`. In an OCB manifest,
use only the module version, for example `v0.155.0`.

## Quick Start

1. Choose the exporter version from the compatibility table.
2. Add the exporter to an OpenTelemetry Collector Builder (`ocb`) manifest. See [Use This Exporter In a Custom Collector](#use-this-exporter-in-a-custom-collector).
3. Configure the Collector with:
   - `namespace`
   - `log_group_id`
   - `auth_type`
   - persistent queue storage using `file_storage`
4. Run the custom Collector with the configuration file shown in [Example Collector Config](#example-collector-config).

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
- Supports OCI config in two ways when using `config_file`:
  - `oci_config_file_path` (+ optional `config_profile`)
  - inline `oci_config` object
- Supports standard Collector exporter settings such as `timeout`, `sending_queue`, and `retry_on_failure`.

## Configuration

### Required Fields

- `namespace`: OCI Log Analytics namespace.
- `log_group_id`: OCI Log Group OCID (used for authorization and routing).
- `auth_type`: `config_file`, `instance_principal`, or `workload_identity`.

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

Do not commit private keys or private-key passphrases to source control. Inject
these values through your deployment's secret-management mechanism. When the
Collector runs on a supported OCI Compute instance or enhanced OKE cluster,
prefer instance principal or workload identity authentication so that a private
API signing key is not included in the Collector configuration.

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

### D) OKE Workload Identity authentication

Replace only the `exporters.oracleobservability` block from the full example
above:

```yaml
exporters:
  oracleobservability:
    auth_type: workload_identity
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
  - gomod: github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter v0.155.0

receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v0.155.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.155.0
  - gomod: go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.155.0

extensions:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.155.0

providers:
  - gomod: go.opentelemetry.io/collector/confmap/provider/envprovider v1.61.0
  - gomod: go.opentelemetry.io/collector/confmap/provider/fileprovider v1.61.0
  - gomod: go.opentelemetry.io/collector/confmap/provider/yamlprovider v1.61.0
```

### 2. Build the binary

```bash
mkdir -p .bin
GOBIN="$PWD/.bin" go install go.opentelemetry.io/collector/cmd/builder@v0.155.0
./.bin/builder --config builder-config.yaml
```

### 3. Run your custom collector

```bash
./_build/otelcol-custom --config /path/to/collector-config.yaml
```

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

To test the exporter in a custom Collector, build a Collector binary with the
OCB manifest shown above and run it with a Collector configuration that uses the
`oracleobservability` exporter.

## OCI IAM Policies

The exporter calls the OCI Log Analytics `UploadOtlpLogs` API. Grant permissions to the principal used by the selected authentication mode, scoped to the compartment that contains the target Log Analytics log group.

Placeholders:

- `<log_group_compartment_ocid>`: OCID of the compartment that contains the target Log Analytics log group.
- `<otel_config_file_user_group>`: IAM group containing the user whose OCI config file or inline `oci_config` is used by the exporter.
- `<instance_ocid>`: OCID of the OCI Compute instance that runs the collector with `auth_type: instance_principal`.
- `otel-exporter-instance-ppl`: dynamic group containing OCI Compute instances that run the exporter with instance principal authentication.
- `<kubernetes_namespace>`: Kubernetes namespace where the Collector pod runs.
- `<service_account_name>`: Kubernetes service account used by the Collector pod.
- `<oke_cluster_ocid>`: OCID of the enhanced OKE cluster that runs the Collector pod.
- `<oke_cluster_compartment_ocid>`: OCID of the compartment that contains the enhanced OKE cluster.
- `<oke_cluster_admin_group>`: IAM group whose administrators create and manage OKE workload mappings.

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

Collector pod identity example:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otel-collector
  namespace: <kubernetes_namespace>
spec:
  selector:
    matchLabels:
      app: otel-collector
  template:
    metadata:
      labels:
        app: otel-collector
    spec:
      serviceAccountName: <service_account_name>
      automountServiceAccountToken: true
      containers:
        - name: otel-collector
          image: "<custom_collector_image>"
          env:
            - name: OCI_RESOURCE_PRINCIPAL_VERSION
              value: "2.2"
            - name: OCI_RESOURCE_PRINCIPAL_REGION
              value: "<oci-region>"
```

Configure these values in the Kubernetes Deployment, not in the
`exporters.oracleobservability` block. The OCI Go SDK uses these environment
variables while creating the OKE Workload Identity signer.

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
compartments, grant the OKE cluster administrators permission to manage workload
mappings and to bind the cluster workloads to the target compartment:

```text
Allow group <oke_cluster_admin_group> to manage cluster-workload-mappings in compartment id <oke_cluster_compartment_ocid>
Allow group <oke_cluster_admin_group> to {CLUSTER_WORKLOAD_COMPARTMENT_BIND, CLUSTER_WORKLOAD_COMPARTMENT_UNBIND} in compartment id <log_group_compartment_ocid>
```

Then create a workload mapping from the Collector's Kubernetes namespace to the
compartment that contains the Log Analytics log group:

```bash
oci ce workload-mapping create \
  --cluster-id <oke_cluster_ocid> \
  --namespace <kubernetes_namespace> \
  --mapped-compartment-id <log_group_compartment_ocid>
```

The workload mapping applies to all service accounts in the mapped Kubernetes
namespace. Keep the upload policy restricted to the Collector's service account
as shown above. For more information, see
[Granting Workloads Access to OCI Resources](https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contenggrantingworkloadaccesstoresources.htm#Example__Using_the_Java_SDK_to_Grant_Application_Workloads_Access_to_OCI_Resources_in_a_Different_Compartment).

### Service policy prerequisite (tenancy-level)

```text
allow service loganalytics to READ loganalytics-features-family in tenancy
```

Notes:

- Scope policies to the compartment containing the target Log Analytics log group.
- If multiple compartments are used, repeat statements per compartment or use a broader compartment/tenancy scope only if required.
- For `config_file`, the policy subject is the IAM group that contains the OCI user, not the config file itself.
- For `instance_principal`, allow time for dynamic group and policy changes to propagate before testing ingestion.
- For `workload_identity`, allow time for OKE workload identity and IAM policy changes to propagate before testing ingestion.
- References:
  - OCI Log Analytics OpenTelemetry upload API: https://docs.oracle.com/en-us/iaas/log-analytics/doc/upload-opentelemetry-logs.html
  - OCI Log Analytics IAM policy details: https://docs.oracle.com/en-us/iaas/log-analytics/doc/iam-policies-upload-open-telemetry-logs.html
  - Log Analytics policy overview: https://docs.oracle.com/en-us/iaas/log-analytics/doc/required-iam-policy.html
  - OCI dynamic groups: https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/managingdynamicgroups.htm
  - OCI instance principals and dynamic group policies: https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/callingservicesfrominstances.htm
  - OKE Workload Identity: https://docs.oracle.com/en-us/iaas/Content/ContEng/Tasks/contenggrantingworkloadaccesstoresources.htm

## Recommendations

- Use `memory_limiter` and `batch` processors in the logs pipeline.
- Start with queue enabled and monitor backpressure.
- Use `instance_principal` when running the Collector on OCI Compute.
- Use `workload_identity` when running the Collector in OKE, especially on OKE Virtual Nodes.
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
