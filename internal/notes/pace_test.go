package notes

import (
	"testing"

	"github.com/danyaa666/smemories/internal/httpx"
)

// The owner route (media/create, T-102) and this route promise the same pace to a phone; one changing alone is a mistake.
func TestSubmitPaceIsThePhoneUploadPace(t *testing.T) {
	if submitPace != httpx.PhoneUploadPace {
		t.Fatalf("submitPace %+v differs from httpx.PhoneUploadPace %+v", submitPace, httpx.PhoneUploadPace)
	}
}
