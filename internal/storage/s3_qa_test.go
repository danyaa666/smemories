package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// QA (T-086): the default chain through the ECS-style container credentials endpoint, and the production defaults end to end.

func TestQAContainerCredentialsEndpoint(t *testing.T) {
	isolateAWS(t)
	var sawAuthToken atomic.Value
	creds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthToken.Store(r.Header.Get("Authorization"))
		exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		fmt.Fprintf(w, `{"AccessKeyId":"AKIDTASKROLE","SecretAccessKey":"task-secret","Token":"task-token","Expiration":%q}`, exp)
	}))
	defer creds.Close()
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", creds.URL+"/creds")
	t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN", "qa-token")
	var hdr atomic.Value
	s3srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr.Store(r.Header.Get("Authorization") + " | " + r.Header.Get("X-Amz-Security-Token"))
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code></Error>`))
	}))
	defer s3srv.Close()
	s := newTestS3(t, S3Config{Endpoint: s3srv.URL}) // empty static keys, defaults for everything else
	if _, err := s.Get(context.Background(), "k"); err != ErrNotFound {
		t.Fatalf("Get: %v", err)
	}
	got, _ := hdr.Load().(string)
	if !strings.Contains(got, "Credential=AKIDTASKROLE/") || !strings.HasSuffix(got, "task-token") {
		t.Fatalf("not signed with the container role: %q", got)
	}
	if v, _ := sawAuthToken.Load().(string); v != "qa-token" {
		t.Fatalf("credentials endpoint got Authorization %q", v)
	}
}

func TestQADefaultsRetries503ExactlyThreeTimes(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	s := newTestS3(t, S3Config{Endpoint: srv.URL, AccessKey: "AKIDQA", SecretKey: "QAsecretKEYvalue"})
	start := time.Now()
	err := s.Put(context.Background(), "k", "text/plain", []byte("x"))
	t.Logf("503 server: %d requests in %v, err=%v", n.Load(), time.Since(start), err)
	if n.Load() != 3 {
		t.Fatalf("want 3 attempts, got %d", n.Load())
	}
	if err != nil && strings.Contains(err.Error(), "QAsecretKEYvalue") {
		t.Fatal("secret in error")
	}
}
