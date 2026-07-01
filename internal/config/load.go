package config

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// envPrefix is the prefix for environment overrides. Convention:
// CASTOR_SECTION__FIELD — the double underscore separates the section from
// the field so single underscores can stay in field names.
//
//	CASTOR_TMDB__API_KEY       → tmdb.api_key
//	CASTOR_NETWORK__TIMEOUT    → network.timeout
//	CASTOR_BROWSER__NO_SANDBOX → browser.no_sandbox
const envPrefix = "CASTOR_"

// Load reads the YAML file at path, overlays CASTOR_* environment variables,
// and validates the result.
func Load(path string) (*Config, error) {
	k := koanf.New(".")

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}
	if err := k.Load(env.Provider(envPrefix, ".", envKey), nil); err != nil {
		return nil, fmt.Errorf("loading environment overrides: %w", err)
	}

	cfg := new(Config)
	if err := k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{
		Tag: "yaml",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				// Parse strings like "30s", "5m" into time.Duration.
				mapstructure.StringToTimeDurationHookFunc(),
				// Split comma-separated env strings into []string.
				mapstructure.StringToSliceHookFunc(","),
				// Honor custom types implementing encoding.TextUnmarshaler.
				mapstructure.TextUnmarshallerHookFunc(),
			),
			WeaklyTypedInput: true,
			Result:           cfg,
			TagName:          "yaml",
		},
	}); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := validator.New().Struct(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return cfg, nil
}

// envKey maps CASTOR_SECTION__FIELD to the koanf key section.field.
func envKey(s string) string {
	s = strings.TrimPrefix(s, envPrefix)
	s = strings.ToLower(s)
	return strings.ReplaceAll(s, "__", ".")
}
