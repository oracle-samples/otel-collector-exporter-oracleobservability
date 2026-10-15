// Copyright (c) 2026, Oracle and/or its affiliates.

// This software is dual-licensed to you under the Universal Permissive License (UPL) 1.0 as shown at https://oss.oracle.com/licenses/upl or Apache License 2.0 as shown at http://www.apache.org/licenses/LICENSE-2.0. You may choose either license.

package oracleobservabilityexporter // import "github.com/oracle-samples/otel-collector-exporter-oracleobservability/oracleobservabilityexporter"

import (
	"errors"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

type AuthenticationType string

const (
	ConfigFile        AuthenticationType = "config_file"
	InstancePrincipal AuthenticationType = "instance_principal"
	WorkloadIdentity  AuthenticationType = "workload_identity"
	ResourcePrincipal AuthenticationType = "resource_principal"
)

type OciConfig struct {
	// The fingerprint of the OCI user.
	FingerPrint configopaque.String `mapstructure:"fingerprint"`

	// The private key of the OCI user.
	PrivateKey configopaque.String `mapstructure:"private_key"`

	// The OCI tenancy OCID.
	Tenancy configopaque.String `mapstructure:"tenancy"`

	// The OCI region.
	Region configopaque.String `mapstructure:"region"`

	// The OCI user OCID.
	User configopaque.String `mapstructure:"user"`
}

// Config defines configuration settings for the Oracle Observability exporter.
type Config struct {
	TimeoutConfig exporterhelper.TimeoutConfig                             `mapstructure:",squash"` // squash ensures fields are correctly decoded in embedded struct
	QueueConfig   configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`
	BackOffConfig configretry.BackOffConfig                                `mapstructure:"retry_on_failure"`

	// The authentication type to use. Supported values are: config_file, instance_principal,
	// workload_identity, or resource_principal. Default is config_file.
	AuthType AuthenticationType `mapstructure:"auth_type"`

	// The OCI tenancy namespace to which the collected log data will be uploaded.
	NamespaceName string `mapstructure:"namespace"`

	// The Log Analytics log group OCID to which the log data will be mapped. This is mainly used for AuthZ purpose.
	LogGroupID string `mapstructure:"log_group_id"`

	// ConfigFilePath is the path to the OCI configuration file. If auth_type is config_file and this field is not provided,
	// the exporter will search for the configuration in the default location ($HOME/.oci/config or the location specified
	// by the OCI_CONFIG_FILE environment variable).
	ConfigFilePath configopaque.String `mapstructure:"oci_config_file_path"`

	// The profile to be used in the OCI configuration file, if auth_type is config_file
	ConfigProfile configopaque.String `mapstructure:"config_profile"`

	// PrivateKeyPassphrase is the passphrase for the private key if encrypted. Optional, only used when private key is password-protected
	PrivateKeyPassphrase configopaque.String `mapstructure:"private_key_passphrase"`

	// An alternative way to provide the content of the OCI Config file, if auth_type is config_file.
	OciConfiguration OciConfig `mapstructure:"oci_config"`
}

// Validate checks if the exporter configuration is valid
var _ component.Config = (*Config)(nil)

// Unmarshal preserves persistent storage by default and allows an explicit
// empty storage string to select an enabled in-memory queue.
// The Collector's confmap decoder discovers this method through the
// confmap.Unmarshaler interface when loading configuration, including validate.
// It is not called per upload. The factory has already populated cfg with
// defaults; conf contains the customer's settings that override those defaults.
func (cfg *Config) Unmarshal(conf *confmap.Conf) error {
	// Work on a map copy, not the customer's YAML or the original Conf. queue is
	// the nested settings map within values, not the runtime queue holding logs.
	values := conf.ToStringMap()
	queue, _ := values["sending_queue"].(map[string]any)
	// A missing/non-map queue yields a nil map, which is safe to read. Require
	// an actual empty string so omission is not mistaken for an in-memory opt-in.
	// Whitespace and other invalid values still go through normal decoding.
	storage, isString := queue["storage"].(string)
	inMemory := isString && storage == ""
	if inMemory {
		// component.ID rejects "". Without removing it, decoding below returns
		// an error before we can set StorageID to nil. Removing the nested key
		// also removes it from values, but does NOT clear cfg's file_storage
		// default. Remember the opt-in in inMemory and clear that default later.
		delete(queue, "storage")
	}
	// This defined type retains Config's fields/tags but not its methods. It is
	// deliberately not an alias (type plainConfig = Config), which would retain
	// Unmarshal.
	type plainConfig Config

	// ToStringMap converted conf into an editable map; NewFromStringMap converts
	// that edited map back into a configuration object. The original conf still
	// contains storage: "" when supplied, so decoding it (or calling ToStringMap
	// on it again) would lose our deletion and cause the empty-ID error.
	inputConfig := confmap.NewFromStringMap(values)

	// Apply the edited settings to cfg. Creating inputConfig above does not
	// update cfg; this decoding step does, using the existing field tags.
	err := inputConfig.Unmarshal((*plainConfig)(cfg))
	if err != nil {
		return err
	}
	// exporterhelper selects its existing in-memory backend for a nil StorageID.
	// HasValue respects enabled:false; we must not re-enable a disabled queue.
	// Queue size, consumers and retry settings are unchanged by this assignment.
	// When storage is omitted, retain the factory's persistent-storage default;
	// missing storage extensions must fail startup rather than fall back to RAM.
	if inMemory && cfg.QueueConfig.HasValue() {
		cfg.QueueConfig.Get().StorageID = nil
	}
	return nil
}

func isOciConfigUsed(ociConfiguration OciConfig) bool {
	if ociConfiguration.FingerPrint != "" || ociConfiguration.PrivateKey != "" || ociConfiguration.Region != "" ||
		ociConfiguration.Tenancy != "" || ociConfiguration.User != "" {
		return true
	}
	return false
}

func (cfg *Config) Validate() error {
	if cfg == nil {
		return errors.New("missing configuration, you must provide a valid configuration for the Oracle Observability exporter")
	}
	if strings.TrimSpace(cfg.NamespaceName) == "" {
		return errors.New("'namespace' is a required field. You may find using OCI Console under Log Analytics → Administration → Service")
	}
	if strings.TrimSpace(cfg.LogGroupID) == "" {
		return errors.New("'log_group_id' is a required field")
	}
	if cfg.AuthType != ConfigFile && cfg.AuthType != InstancePrincipal && cfg.AuthType != ResourcePrincipal && cfg.AuthType != WorkloadIdentity {
		return errors.New("invalid 'auth_type', supported values are 'config_file', 'instance_principal', 'workload_identity', and 'resource_principal'")
	}

	isOciConfigUsed := isOciConfigUsed(cfg.OciConfiguration)

	if cfg.AuthType != ConfigFile {
		if isOciConfigUsed {
			return errors.New("'oci_config' field is only applicable when 'auth_type' is set to 'config_file'")
		}
		if cfg.ConfigFilePath != "" {
			return errors.New("'oci_config_file_path' field is only applicable when 'auth_type' is set to 'config_file'")
		}
		if cfg.ConfigProfile != "" {
			return errors.New("'config_profile' field is only applicable when 'auth_type' is set to 'config_file'")
		}
		if cfg.PrivateKeyPassphrase != "" {
			return errors.New("'private_key_passphrase' field is only applicable when 'auth_type' is set to 'config_file'")
		}
	}

	if isOciConfigUsed {
		if cfg.OciConfiguration.FingerPrint == "" {
			return errors.New("'fingerprint' can not be empty")
		}
		if cfg.OciConfiguration.PrivateKey == "" {
			return errors.New("'private_key' can not be empty")
		}
		if cfg.OciConfiguration.Region == "" {
			return errors.New("'region' can not be empty")
		}
		if cfg.OciConfiguration.Tenancy == "" {
			return errors.New("'tenancy' can not be empty")
		}
		if cfg.OciConfiguration.User == "" {
			return errors.New("'user' can not be empty")
		}
	}

	return nil
}
