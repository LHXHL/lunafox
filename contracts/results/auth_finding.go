package results

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// AuthFinding records a verified Web authentication exposure. It intentionally
// has no password, raw tool output, or arbitrary evidence field: zombie's
// original JSON includes plaintext passwords and must never enter this result.
type AuthFinding struct {
	URL     string `json:"url"`
	Service string `json:"service"`
	Kind    string `json:"kind"`
	Account string `json:"account,omitempty"`
}

func validateAuthFinding(item AuthFinding) error {
	if _, err := ValidateObservedAssetURL(item.URL); err != nil {
		return fmt.Errorf("auth finding url is invalid: %w", err)
	}
	parsed, err := url.Parse(item.URL)
	if err != nil || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("auth finding url must be a Web origin")
	}
	if item.Service != "http" && item.Service != "https" {
		return errors.New("auth finding service must be http or https")
	}
	if parsed.Scheme != item.Service {
		return errors.New("auth finding service conflicts with url scheme")
	}
	if item.Kind != "valid_credential" && item.Kind != "anonymous_access" {
		return errors.New("auth finding kind is invalid")
	}
	if !utf8.ValidString(item.Account) || len(item.Account) > 128 || strings.TrimSpace(item.Account) != item.Account {
		return errors.New("auth finding account is invalid")
	}
	if item.Kind == "anonymous_access" && item.Account != "" {
		return errors.New("anonymous auth finding must not contain an account")
	}
	if item.Kind == "valid_credential" && item.Account == "" {
		return errors.New("credential auth finding requires an account")
	}
	return nil
}

func EncodeAuthFinding(item AuthFinding) (string, error) {
	if err := validateAuthFinding(item); err != nil {
		return "", err
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("marshal auth finding result: %w", err)
	}
	return string(payload), nil
}
