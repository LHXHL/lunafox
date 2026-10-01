package results

import (
	"strings"
	"testing"
)

func TestAuthFindingRoundTripAndClosedWire(t *testing.T) {
	item := AuthFinding{URL: "https://auth.example.test:8443/", Service: "https", Kind: "valid_credential", Account: "test-user"}
	payload, err := EncodeAuthFinding(item)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeAuthFindingItems([]string{payload})
	if err != nil || len(decoded) != 1 || decoded[0] != item {
		t.Fatalf("decoded = %+v, err = %v", decoded, err)
	}
	if _, err := ValidateCanonicalBatch(ResultKindSecurityAuthFinding, [][]byte{[]byte(payload)}, DefaultBatchLimits()); err != nil {
		t.Fatal(err)
	}
	withPassword := strings.TrimSuffix(payload, "}") + `,"password":"secret"}`
	if _, err := DecodeAuthFindingItems([]string{withPassword}); err == nil {
		t.Fatal("auth finding wire accepted plaintext password")
	}
}

func TestAuthFindingRejectsAmbiguousOrInvalidObservations(t *testing.T) {
	base := AuthFinding{URL: "http://auth.example.test/", Service: "http", Kind: "valid_credential", Account: "test-user"}
	cases := []AuthFinding{
		{URL: "http://auth.example.test/path", Service: base.Service, Kind: base.Kind, Account: base.Account},
		{URL: base.URL, Service: "https", Kind: base.Kind, Account: base.Account},
		{URL: base.URL, Service: base.Service, Kind: "unknown", Account: base.Account},
		{URL: base.URL, Service: base.Service, Kind: base.Kind},
		{URL: base.URL, Service: base.Service, Kind: "anonymous_access", Account: base.Account},
	}
	for _, item := range cases {
		if _, err := EncodeAuthFinding(item); err == nil {
			t.Fatalf("accepted invalid finding: %+v", item)
		}
	}
}
