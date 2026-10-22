package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/danyaa666/smemories/internal/legal"
)

// T-111: register refuses a missing or old Terms version before it touches anything (a zero Service would panic otherwise).
func TestRegisterRefusesWrongTermsVersionFirst(t *testing.T) {
	for _, v := range []string{"", "2020-01-01", legal.CurrentVersion + " "} {
		_, _, err := (&Service{}).Register(context.Background(), "ip", "ua", "a@example.com", testPWUnit, "A", "en", v)
		if !errors.Is(err, errTermsAtRegister) {
			t.Errorf("version %q: %v", v, err)
		}
	}
}

const testPWUnit = "Tr0ub4dor&3-fake-test-password"

func TestTermsOf(t *testing.T) {
	if got := termsOf(&User{}); got.AcceptedVersion != nil || !got.Required || got.CurrentVersion != legal.CurrentVersion {
		t.Errorf("never accepted: %+v", got)
	}
	if got := termsOf(&User{TermsVersion: "2020-01-01"}); got.AcceptedVersion == nil || *got.AcceptedVersion != "2020-01-01" || !got.Required {
		t.Errorf("old version: %+v", got)
	}
	if got := termsOf(&User{TermsVersion: legal.CurrentVersion}); got.Required {
		t.Errorf("current version: %+v", got)
	}
}
