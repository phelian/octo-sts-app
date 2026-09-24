// Copyright 2024 Chainguard, Inc.
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v88/github"
)

func discoveryClient(t *testing.T, handler http.Handler) *github.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := github.NewClient(github.WithHTTPClient(srv.Client()), github.WithEnterpriseURLs(srv.URL, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestPolicyFilesFromPRPaginatesAndChecksSnapshot(t *testing.T) {
	gets := 0
	client := discoveryClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/o/r/pulls/7":
			gets++
			json.NewEncoder(w).Encode(&github.PullRequest{
				Head: &github.PullRequestBranch{SHA: new("head")}, Base: &github.PullRequestBranch{SHA: new("base")}, ChangedFiles: new(101),
			})
		case "/api/v3/repos/o/r/pulls/7/files":
			if r.URL.Query().Get("per_page") != "100" {
				t.Errorf("per_page = %q", r.URL.Query().Get("per_page"))
			}
			if r.URL.Query().Get("page") == "1" {
				files := make([]*github.CommitFile, 100)
				for i := range files {
					files[i] = &github.CommitFile{Filename: new(fmt.Sprintf("other/%03d", i)), Status: new("modified")}
				}
				w.Header().Set("Link", fmt.Sprintf("<http://%s%s?page=2&per_page=100>; rel=\"next\"", r.Host, r.URL.Path))
				json.NewEncoder(w).Encode(files)
			} else {
				json.NewEncoder(w).Encode([]*github.CommitFile{{Filename: new(".github/chainguard/policy.sts.yaml"), Status: new("added")}})
			}
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	files, err := (&Validator{}).policyFilesFromPR(context.Background(), client, "o", "r", 7, "head")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != ".github/chainguard/policy.sts.yaml" {
		t.Fatalf("files = %v", files)
	}
	if gets != 2 {
		t.Fatalf("snapshot reads = %d, want 2", gets)
	}
}

func TestPolicyFilesFromPRRejectsChangingSnapshot(t *testing.T) {
	gets := 0
	client := discoveryClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/o/r/pulls/7":
			gets++
			base := "base"
			if gets == 2 {
				base = "new-base"
			}
			json.NewEncoder(w).Encode(&github.PullRequest{
				Head: &github.PullRequestBranch{SHA: new("head")}, Base: &github.PullRequestBranch{SHA: &base}, ChangedFiles: new(1),
			})
		case "/api/v3/repos/o/r/pulls/7/files":
			json.NewEncoder(w).Encode([]*github.CommitFile{{Filename: new(".github/chainguard/policy.sts.yaml"), Status: new("modified")}})
		default:
			http.NotFound(w, r)
		}
	}))
	_, err := (&Validator{}).policyFilesFromPR(context.Background(), client, "o", "r", 7, "head")
	if err == nil || !strings.Contains(err.Error(), "changed during file listing") {
		t.Fatalf("error = %v", err)
	}
}

func TestPolicyTreeSnapshotRejectsTruncatedTree(t *testing.T) {
	client := discoveryClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/o/r/git/commits/head":
			json.NewEncoder(w).Encode(&github.Commit{Tree: &github.Tree{SHA: new("tree")}})
		case "/api/v3/repos/o/r/git/trees/tree":
			json.NewEncoder(w).Encode(&github.Tree{Truncated: new(true), Entries: []*github.TreeEntry{}})
		default:
			http.NotFound(w, r)
		}
	}))
	_, err := (&Validator{}).policyTreeSnapshot(context.Background(), client, "o", "r", "head")
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error = %v", err)
	}
}
