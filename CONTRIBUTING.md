# Contributing to this repository

We welcome your contributions! There are multiple ways to contribute.

## Opening issues

For bugs or enhancement requests, please file a GitHub issue unless it's
security related. When filing a bug remember that the better written the bug is,
the more likely it is to be fixed. If you think you've found a security
vulnerability, do not raise a GitHub issue and follow the instructions in our
[security policy](./SECURITY.md).

## Contributing code

We welcome your code contributions. Before submitting code via a pull request,
you will need to have signed the [Oracle Contributor Agreement][OCA] (OCA) and
your commits need to include the following line using the name and e-mail
address you used to sign the OCA:

```text
Signed-off-by: Your Name <you@example.org>
```

This can be automatically added to pull requests by committing with `--sign-off`
or `-s`, e.g.

```text
git commit --signoff
```

Only pull requests from committers that can be verified as having signed the OCA
can be accepted.

## Pull request process

1. Ensure there is an issue created to track and discuss the fix or enhancement
   you intend to submit.
1. Fork this repository.
1. Create a branch in your fork to implement the changes. We recommend using
   the issue number as part of your branch name, e.g. `1234-fixes`.
1. Ensure that any documentation is updated with the changes that are required
   by your change.
1. Ensure that any samples are updated if the base image has been changed.
1. Submit the pull request. *Do not leave the pull request blank*. Explain exactly
   what your changes are meant to do and provide simple steps on how to validate.
   your changes. Ensure that you reference the issue you created as well.
1. We will assign the pull request to 2-3 people for review before it is merged.

## Local validation

You can validate exporter changes from your local checkout before publishing a
release tag.

### Build a local custom collector with your working tree

Use `path` in the OpenTelemetry Collector Builder manifest to point to your
local checkout:

```yaml
exporters:
  - gomod: github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter v0.155.0
    path: /absolute/path/to/otel-collector-exporter-oracleobservability/oracleobservabilityexporter
```

Then run:

```bash
builder --config builder-config.yaml
```

This compiles a collector binary using your local code.

### Run a smoke test pipeline

The easiest local smoke test is to read from a local test log file with the
`filelogreceiver`.

Make sure your OCB manifest includes `filelogreceiver`, `batchprocessor`,
`filestorage`, and this exporter:

```yaml
receivers:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver v0.155.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.155.0

extensions:
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.155.0

exporters:
  - gomod: github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter v0.155.0
```

Use a Collector configuration similar to this:

```yaml
receivers:
  filelog:
    include:
      - /tmp/oracleobservability-smoke.log
    start_at: beginning

processors:
  batch:
    timeout: 30s
    send_batch_size: 1024

exporters:
  oracleobservability:
    auth_type: config_file
    namespace: "<oci-loganalytics-namespace>"
    log_group_id: "ocid1.loganalyticsloggroup.oc1..<unique_id>"
    oci_config_file_path: "/path/to/oci/config"
    config_profile: "DEFAULT"

extensions:
  file_storage:
    directory: "<path_to_storage>"
    create_directory: true

service:
  extensions: [file_storage]
  pipelines:
    logs:
      receivers: [filelog]
      processors: [batch]
      exporters: [oracleobservability]
```

Start the Collector, then append a test line:

```bash
echo "oracleobservability smoke test $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> /tmp/oracleobservability-smoke.log
```

Verify that:

- the Collector starts successfully
- the file log is read by the Collector
- logs are exported to OCI Log Analytics
- retry and queue behavior is visible under transient failures

### Run local quality gates

```bash
go -C oracleobservabilityexporter test ./...
go -C oracleobservabilityexporter test -race ./...
go -C oracleobservabilityexporter vet ./...
```

## Code of conduct

Follow the [Golden Rule](https://en.wikipedia.org/wiki/Golden_Rule). If you'd
like more specific guidelines, see the [Contributor Covenant Code of Conduct][COC].

[OCA]: https://oca.opensource.oracle.com
[COC]: https://www.contributor-covenant.org/version/1/4/code-of-conduct/
