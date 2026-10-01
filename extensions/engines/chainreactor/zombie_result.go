package chainreactor

import (
	"errors"
	"net"
	"net/url"
	"strconv"

	"github.com/yyhuni/lunafox/contracts/results"
)

// ZombieAuthResult converts a redacted Web finding to the closed Server result.
// The upstream password and raw JSON are unavailable at this boundary.
func ZombieAuthResult(finding AuthFinding) (results.AuthFinding, error) {
	if finding.Service != "http" && finding.Service != "https" {
		return results.AuthFinding{}, errors.New("zombie authentication result is not a Web service")
	}
	address := finding.Host
	if address == "" {
		address = finding.IP
	}
	if address == "" || finding.Port < 1 || finding.Port > 65535 {
		return results.AuthFinding{}, errors.New("zombie authentication result endpoint is incomplete")
	}
	item := results.AuthFinding{
		URL:     (&url.URL{Scheme: finding.Service, Host: net.JoinHostPort(address, strconv.Itoa(finding.Port)), Path: "/"}).String(),
		Service: finding.Service,
		Kind:    finding.Kind,
		Account: finding.Account,
	}
	if _, err := results.EncodeAuthFinding(item); err != nil {
		return results.AuthFinding{}, err
	}
	return item, nil
}
