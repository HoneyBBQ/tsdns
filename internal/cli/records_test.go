package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecordsAdd_UsesAdminAPI(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/records" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		gotAuth = r.Header.Get("Authorization")

		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		_ = json.Unmarshal(b, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	var configPath string
	cmd := newRecordsCommand(&configPath)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"add",
		"--api-url", ts.URL,
		"--token", "secret",
		"--domain", "demo.example.com",
		"--target", "1.2.3.4",
		"--port", "9987",
		"--instance-id", "42",
	})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if gotAuth != "Bearer secret" {
		t.Fatalf("unexpected Authorization header: %q", gotAuth)
	}
	targets, ok := gotBody["targets"].([]any)
	if !ok || len(targets) != 1 || targets[0] != "1.2.3.4:9987" {
		t.Fatalf("unexpected json body targets: %#v", gotBody["targets"])
	}
	if gotBody["domain"] != "demo.example.com" {
		t.Fatalf("unexpected json body domain: %#v", gotBody["domain"])
	}
}

func TestRecordsList_PrintsJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/records" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"Domain":"demo.example.com","Target":["1.2.3.4"],"Port":9987,"InstanceID":0}]`))
	}))
	defer ts.Close()

	var configPath string
	cmd := newRecordsCommand(&configPath)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"list",
		"--api-url", ts.URL,
	})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(out.String(), "demo.example.com") {
		t.Fatalf("expected output to contain domain, got: %s", out.String())
	}
}
