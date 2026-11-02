package httpapi

import (
	"net/http/httptest"
	"testing"
)

func mustStatus(t *testing.T, r *httptest.ResponseRecorder, status int) {
	t.Helper()
	if r.Code != status {
		t.Fatal(r.Code, r.Body.String())
	}
}
