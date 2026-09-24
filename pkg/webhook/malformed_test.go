// Copyright 2024 Chainguard, Inc.
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v88/github"
)

func TestServeHTTPRejectsMalformedSignedEvents(t *testing.T) {
	secret := []byte("test-secret")
	srv := httptest.NewServer(&Validator{WebhookSecret: [][]byte{secret}})
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name, event, body string
	}{
		{"missing PR", "pull_request", `{}`},
		{"missing PR head", "pull_request", `{"number":1,"pull_request":{},"repository":{"name":"r","owner":{"login":"o"}},"installation":{"id":1}}`},
		{"missing push commits", "push", `{"before":"before","after":"after","repository":{"name":"r","owner":{"login":"o"}},"installation":{"id":1}}`},
		{"null push commit", "push", `{"before":"before","after":"after","commits":[null],"repository":{"name":"r","owner":{"login":"o"}},"installation":{"id":1}}`},
		{"missing check suite", "check_suite", `{}`},
		{"null associated PR", "check_suite", `{"check_suite":{"head_sha":"head","before":"before","pull_requests":[null]},"repository":{"name":"r","owner":{"login":"o"}},"installation":{"id":1}}`},
		{"missing check run", "check_run", `{}`},
		{"incomplete bot check run", "check_run", `{"sender":{"login":"other[bot]"},"check_run":{"head_sha":"head","check_suite":{}},"repository":{"name":"r","owner":{"login":"o"}},"installation":{"id":1}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(github.SHA256SignatureHeader, signature(secret, body))
			req.Header.Set(HeaderEvent, tc.event)
			req.Header.Set("Content-Type", "application/json")
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
}
