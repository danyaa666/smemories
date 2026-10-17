package mailer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/mailer/mailertest"
)

// sesStub is a local stand-in for the SESv2 HTTP API: it records the requests and answers as told.
type sesStub struct {
	srv *httptest.Server

	mu       sync.Mutex
	behave   mailertest.Behaviour
	requests []stubRequest
	release  chan struct{} // closes the hanging requests at cleanup
}

type stubRequest struct {
	Method, Path, Authorization, AmzDate string
	Body                                 string
}

func newSESStub(t *testing.T) *sesStub {
	t.Helper()
	s := &sesStub{release: make(chan struct{})}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(func() { close(s.release); s.srv.Close() })
	return s
}

func (s *sesStub) handle(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, stubRequest{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Amz-Date"), string(b)})
	behave := s.behave
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, errType, msg string) {
		w.Header().Set("X-Amzn-Errortype", errType)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
	}
	switch behave {
	case mailertest.Healthy:
		_, _ = io.WriteString(w, `{"MessageId":"0000-stub"}`)
	case mailertest.Throttled:
		fail(http.StatusTooManyRequests, "TooManyRequestsException", "Too many requests for leak.check.student@school-mail.invalid")
	case mailertest.ServerError:
		fail(http.StatusInternalServerError, "InternalServiceErrorException", "boom leak.check.student@school-mail.invalid")
	case mailertest.Refused:
		fail(http.StatusBadRequest, "MessageRejected", "Email address is not verified: leak.check.student@school-mail.invalid")
	case mailertest.Hang:
		select {
		case <-r.Context().Done():
		case <-s.release:
		}
	}
}

func (s *sesStub) set(b mailertest.Behaviour) { s.mu.Lock(); s.behave = b; s.mu.Unlock() }

func (s *sesStub) all() []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubRequest(nil), s.requests...)
}

// awsEnv isolates the SDK from the developer's machine and gives it environment credentials, the way the default chain finds
// them on a task.
func awsEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDSTUBSTUBSTUB")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "stub-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent/config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent/credentials")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// fast timeouts so the hanging and failing stubs finish quickly; attempts stay at the default of 3.
func sesCfg(stub *sesStub) mailer.SESConfig {
	return mailer.SESConfig{
		From: "SMemories <no-reply@mail.smemories.io>", ConfigurationSet: "smem-prod", Endpoint: stub.srv.URL,
		ResponseHeaderTimeout: 200 * time.Millisecond, OperationTimeout: 5 * time.Second, MaxBackoff: 10 * time.Millisecond,
	}
}

func newSES(t *testing.T, cfg mailer.SESConfig) (*mailer.SES, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	m, err := mailer.NewSES(cfg, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return m, &logs
}

// The conformance suite, run by SES against the stub.
func TestSESConformance(t *testing.T) {
	awsEnv(t)
	mailertest.Run(t, func(t *testing.T) mailertest.Fixture {
		stub := newSESStub(t)
		m, logs := newSES(t, sesCfg(stub))
		return mailertest.Fixture{
			Mailer: m,
			Logs:   logs.String,
			Saw: func() string {
				var sb strings.Builder
				for _, r := range stub.all() {
					sb.WriteString(r.Body)
				}
				return sb.String()
			},
			Behave: stub.set,
		}
	})
}

// AC1: the request has the shape SESv2 expects and is SigV4-signed with the credentials of the default chain.
func TestSESRequestShape(t *testing.T) {
	awsEnv(t)
	stub := newSESStub(t)
	m, _ := newSES(t, sesCfg(stub))
	msg := mailer.Message{To: "student@school-mail.invalid", Subject: "Mã xác minh", Text: "plain 123456", HTML: "<h1>html 123456</h1>"}
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	reqs := stub.all()
	if len(reqs) != 1 {
		t.Fatalf("%d requests, want 1", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/v2/email/outbound-emails" {
		t.Errorf("request %s %s", r.Method, r.Path)
	}
	if !strings.HasPrefix(r.Authorization, "AWS4-HMAC-SHA256 Credential=AKIDSTUBSTUBSTUB/") || !strings.Contains(r.Authorization, "/eu-west-1/ses/aws4_request") ||
		!strings.Contains(r.Authorization, "Signature=") || r.AmzDate == "" {
		t.Errorf("not SigV4-signed with the environment credentials: %q (date %q)", r.Authorization, r.AmzDate)
	}
	var got struct {
		FromEmailAddress     string
		ConfigurationSetName string
		Destination          struct{ ToAddresses []string }
		Content              struct {
			Simple struct {
				Subject struct{ Data, Charset string }
				Body    struct {
					Text, Html struct{ Data, Charset string }
				}
			}
		}
	}
	if err := json.Unmarshal([]byte(r.Body), &got); err != nil {
		t.Fatal(err)
	}
	s := got.Content.Simple
	switch {
	case got.FromEmailAddress != "SMemories <no-reply@mail.smemories.io>",
		got.ConfigurationSetName != "smem-prod",
		len(got.Destination.ToAddresses) != 1 || got.Destination.ToAddresses[0] != msg.To,
		s.Subject.Data != msg.Subject || s.Subject.Charset != "UTF-8",
		s.Body.Text.Data != msg.Text || s.Body.Html.Data != msg.HTML || s.Body.Text.Charset != "UTF-8" || s.Body.Html.Charset != "UTF-8":
		t.Errorf("request body is not what was sent: %s", r.Body)
	}
}

func TestSESWithoutConfigurationSetOrHTML(t *testing.T) {
	awsEnv(t)
	stub := newSESStub(t)
	cfg := sesCfg(stub)
	cfg.ConfigurationSet = ""
	m, _ := newSES(t, cfg)
	if err := m.Send(context.Background(), mailer.Message{To: "a@school-mail.invalid", Subject: "s", Text: "t"}); err != nil {
		t.Fatal(err)
	}
	if b := stub.all()[0].Body; strings.Contains(b, "ConfigurationSetName") || strings.Contains(b, "Html") {
		t.Errorf("optional parts should be absent: %s", b)
	}
}

// AC1: which SES answers become which error, and how many attempts are made.
func TestSESErrorMapping(t *testing.T) {
	awsEnv(t)
	for _, c := range []struct {
		name         string
		status       int
		errType      string
		want         error
		wantAttempts int
	}{
		{"429 throttling", 429, "TooManyRequestsException", mailer.ErrUnavailable, 3},
		{"throttling as 400", 400, "ThrottlingException", mailer.ErrUnavailable, 3},
		{"daily quota", 400, "LimitExceededException", mailer.ErrUnavailable, 3},
		{"500", 500, "InternalServiceErrorException", mailer.ErrUnavailable, 3},
		{"503 without a type", 503, "", mailer.ErrUnavailable, 3},
		{"MessageRejected", 400, "MessageRejected", mailer.ErrRejected, 1},
		{"AccountSuspendedException", 400, "AccountSuspendedException", mailer.ErrRejected, 1},
		{"SendingPausedException", 400, "SendingPausedException", mailer.ErrRejected, 1},
		{"unrelated 400 (a misconfiguration)", 400, "NotFoundException", nil, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			n := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				n++
				mu.Unlock()
				if c.errType != "" {
					w.Header().Set("X-Amzn-Errortype", c.errType)
				}
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, `{"message":"for student@school-mail.invalid"}`)
			}))
			defer srv.Close()
			cfg := sesCfg(&sesStub{srv: srv})
			m, logs := newSES(t, cfg)
			err := m.Send(context.Background(), mailer.Message{To: "student@school-mail.invalid", Subject: "s", Text: "t"})
			if err == nil {
				t.Fatal("Send succeeded")
			}
			if c.want != nil && !errors.Is(err, c.want) {
				t.Errorf("Send = %v, want %v", err, c.want)
			}
			if c.want == nil && (errors.Is(err, mailer.ErrUnavailable) || errors.Is(err, mailer.ErrRejected)) {
				t.Errorf("a misconfiguration must be neither class: %v", err)
			}
			if n != c.wantAttempts {
				t.Errorf("%d attempts, want %d", n, c.wantAttempts)
			}
			for _, s := range []string{err.Error(), logs.String()} {
				if strings.Contains(s, "student@school-mail.invalid") {
					t.Errorf("the recipient leaked: %s", s)
				}
			}
		})
	}
}

// AC1: a blackholed endpoint (accepts, never answers) is cut by the timeouts, within 3 attempts.
func TestSESBlackholeIsBounded(t *testing.T) {
	awsEnv(t)
	stub := newSESStub(t)
	stub.set(mailertest.Hang)
	m, _ := newSES(t, sesCfg(stub))
	start := time.Now()
	err := m.Send(context.Background(), mailer.Message{To: "a@school-mail.invalid", Subject: "s", Text: "t"})
	took := time.Since(start)
	if !errors.Is(err, mailer.ErrUnavailable) {
		t.Fatalf("Send = %v, want ErrUnavailable", err)
	}
	if n := len(stub.all()); n > 3 || n < 1 {
		t.Errorf("%d attempts, want at most 3", n)
	}
	if took > 3*time.Second { // 3 x 200 ms header timeout + 2 short backoffs
		t.Errorf("took %s", took)
	}
}

// The operation timeout cuts the whole call even when every attempt would still be waiting.
func TestSESOperationTimeout(t *testing.T) {
	awsEnv(t)
	stub := newSESStub(t)
	stub.set(mailertest.Hang)
	cfg := sesCfg(stub)
	cfg.ResponseHeaderTimeout = 10 * time.Second
	cfg.OperationTimeout = 300 * time.Millisecond
	m, _ := newSES(t, cfg)
	start := time.Now()
	err := m.Send(context.Background(), mailer.Message{To: "a@school-mail.invalid", Subject: "s", Text: "t"})
	if !errors.Is(err, mailer.ErrUnavailable) || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Fatalf("Send = %v after %s", err, time.Since(start))
	}
}

func TestSESConnectionRefusedIsUnavailable(t *testing.T) {
	awsEnv(t)
	stub := newSESStub(t)
	cfg := sesCfg(stub)
	stub.srv.Close() // the port now refuses connections
	m, _ := newSES(t, cfg)
	err := m.Send(context.Background(), mailer.Message{To: "a@school-mail.invalid", Subject: "s", Text: "t"})
	if !errors.Is(err, mailer.ErrUnavailable) {
		t.Fatalf("Send = %v, want ErrUnavailable", err)
	}
}

func TestNewSESRefusals(t *testing.T) {
	awsEnv(t)
	if _, err := mailer.NewSES(mailer.SESConfig{From: "not an address"}, slog.New(slog.DiscardHandler)); err == nil {
		t.Error("a bad From was accepted")
	}
	if _, err := mailer.NewSES(mailer.SESConfig{}, slog.New(slog.DiscardHandler)); err == nil {
		t.Error("an empty From was accepted")
	}
	t.Setenv("AWS_REGION", "")
	if _, err := mailer.NewSES(mailer.SESConfig{From: "a@mail.smemories.io"}, slog.New(slog.DiscardHandler)); err == nil || !strings.Contains(err.Error(), "AWS_REGION") {
		t.Errorf("a missing region: %v", err)
	}
}
