package mailer

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/smithy-go"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Timeouts and retries of the SES client. The first three and the attempts are the S3 client's (storage.Default*, T-086): the
// AWS default is about 25 s to give up on a refused connection plus silent retries. A mail is small, so the whole call is
// bounded by 30 s and the retry backoff by 2 s (a request that waits on it is a person on the resend screen).
const (
	DefaultDialTimeout           = 3 * time.Second
	DefaultTLSHandshakeTimeout   = 5 * time.Second
	DefaultResponseHeaderTimeout = 15 * time.Second
	DefaultOperationTimeout      = 30 * time.Second // one Send end to end, retries included
	DefaultMaxAttempts           = 3                // first try included
	DefaultMaxBackoff            = 2 * time.Second
)

const charset = "UTF-8" // subjects and bodies are Vietnamese too

// SESConfig configures the Amazon SES (SESv2) mailer. The region and the credentials come from the AWS default configuration
// (AWS_REGION, the ECS task role); a zero timeout takes the Default* value above.
//
// SESConfig is passed by value on purpose: start-up settings, read once.
type SESConfig struct {
	From             string // SMEM_MAIL_FROM, "Name <address>" allowed; set by New
	ConfigurationSet string // SMEM_SES_CONFIGURATION_SET: optional, adds reputation metrics and events

	// Endpoint points the client at a stub of the SESv2 API in tests; empty in production.
	Endpoint string

	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	OperationTimeout      time.Duration
	MaxAttempts           int
	MaxBackoff            time.Duration
}

// SES sends through the SESv2 SendEmail API.
type SES struct {
	c       *sesv2.Client
	from    string
	set     *string
	opLimit time.Duration
	logger  *slog.Logger
}

// orDefault returns v, or def when v is zero.
func orDefault[T ~int | ~int64](v, def T) T {
	if v == 0 {
		return def
	}
	return v
}

// NewSES builds the client; it does not contact SES. The credential chain resolves on the first send, so a task without a role
// fails there, not here. It refuses a missing From, a missing region and a broken AWS configuration.
func NewSES(cfg SESConfig, logger *slog.Logger) (*SES, error) {
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, apperr.New(apperr.Internal, "mailer: SMEM_MAIL_FROM must be a valid address such as \"SMemories <no-reply@mail.example.com>\"")
	}
	dc, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "mailer: AWS default configuration")
	}
	if dc.Region == "" {
		return nil, apperr.New(apperr.Internal, "mailer: no AWS region: set AWS_REGION (the ECS task has it)")
	}
	httpc := awshttp.NewBuildableClient().
		WithDialerOptions(func(d *net.Dialer) { d.Timeout = orDefault(cfg.DialTimeout, DefaultDialTimeout) }).
		WithTransportOptions(func(t *http.Transport) {
			t.TLSHandshakeTimeout = orDefault(cfg.TLSHandshakeTimeout, DefaultTLSHandshakeTimeout)
			t.ResponseHeaderTimeout = orDefault(cfg.ResponseHeaderTimeout, DefaultResponseHeaderTimeout)
		})
	c := sesv2.NewFromConfig(dc, func(o *sesv2.Options) {
		o.HTTPClient = httpc
		o.Retryer = retry.NewStandard(func(so *retry.StandardOptions) {
			so.MaxAttempts = orDefault(cfg.MaxAttempts, DefaultMaxAttempts)
			so.Backoff = retry.NewExponentialJitterBackoff(orDefault(cfg.MaxBackoff, DefaultMaxBackoff))
		})
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	s := &SES{c: c, from: cfg.From, opLimit: orDefault(cfg.OperationTimeout, DefaultOperationTimeout), logger: logger}
	if cfg.ConfigurationSet != "" {
		s.set = aws.String(cfg.ConfigurationSet)
	}
	logger.Info("mailer ready", "provider", ProviderSES, "region", dc.Region, "configuration_set", cfg.ConfigurationSet)
	return s, nil
}

// Send emails m as multipart/alternative (text and HTML). SendEmail is not idempotent: a retry after a lost answer can deliver the same
// message twice, which for a code email is harmless. Failures are ErrUnavailable or ErrRejected (see classifySES); the log
// line has the class and the SES error code only, never the address or the text.
func (s *SES) Send(ctx context.Context, m Message) error {
	ctx, cancel := context.WithTimeout(ctx, s.opLimit)
	defer cancel()
	body := &types.Body{Text: &types.Content{Data: aws.String(m.Text), Charset: aws.String(charset)}}
	if m.HTML != "" {
		body.Html = &types.Content{Data: aws.String(m.HTML), Charset: aws.String(charset)}
	}
	_, err := s.c.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress:     &s.from,
		Destination:          &types.Destination{ToAddresses: []string{m.To}},
		ConfigurationSetName: s.set,
		Content: &types.EmailContent{Simple: &types.Message{
			Subject: &types.Content{Data: aws.String(m.Subject), Charset: aws.String(charset)},
			Body:    body,
		}},
	})
	if err == nil {
		return nil
	}
	err = classifySES(ctx, err)
	level := slog.LevelWarn
	if Class(err) == ClassConfig { // only the operator can fix it, and every send fails until then
		level = slog.LevelError
	}
	s.logger.Log(ctx, level, "mailer: send failed", "provider", ProviderSES, "class", Class(err), "reason", err.Error())
	return err
}

// errFailed is any failure that is neither of the two the caller handles: a misconfiguration or bad credentials.
var errFailed = apperr.New(apperr.Internal, "mail send failed")

// Throttling error codes SES (and the gateway in front of it) answers with a 4xx.
var throttleCodes = map[string]bool{
	"TooManyRequestsException": true, "LimitExceededException": true, "ThrottlingException": true, "Throttling": true, "RequestLimitExceeded": true,
}

// notVerifiedTexts are the parts of the MessageRejected text SES gives for an unverified sender identity and, while the account is in the
// sandbox, for an unverified recipient: "Email address is not verified. The following identities failed the check in region ..." and
// "Identity ... is not verified". The text is only matched, never logged (it holds the address).
var notVerifiedTexts = []string{"is not verified", "are not verified"}

func isNotVerified(msg string) bool {
	for _, t := range notVerifiedTexts {
		if strings.Contains(msg, t) {
			return true
		}
	}
	return false
}

// classifySES maps an SDK error to ErrUnavailable (throttled, 5xx, timeout, network; also, as ClassConfig, an account that is
// suspended or paused and a sender or sandbox recipient that is not verified: the operator must act, so it is loud and counted, not a
// per-address refusal), ErrRejected (this address is refused for good) or errFailed. It is the only place that knows SES error names.
func classifySES(ctx context.Context, err error) error {
	var (
		mr  *types.MessageRejected
		sus *types.AccountSuspendedException
		pau *types.SendingPausedException
		ae  smithy.APIError
		re  *awshttp.ResponseError
		ne  net.Error
	)
	code := "unknown"
	if errors.As(err, &ae) {
		code = ae.ErrorCode()
	}
	switch {
	case errors.As(err, &sus), errors.As(err, &pau):
		return classifyAs(ErrUnavailable, ClassConfig, code, err)
	case errors.As(err, &mr):
		if isNotVerified(aws.ToString(mr.Message)) {
			return classifyAs(ErrUnavailable, ClassConfig, code+" not verified", err)
		}
		return classify(ErrRejected, code, err)
	case errors.Is(ctx.Err(), context.Canceled):
		return classify(errFailed, "canceled", err)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return classify(ErrUnavailable, "timeout", err)
	case throttleCodes[code]:
		return classify(ErrUnavailable, code, err)
	case errors.As(err, &re) && (re.HTTPStatusCode() == http.StatusTooManyRequests || re.HTTPStatusCode() >= http.StatusInternalServerError):
		return classify(ErrUnavailable, code, err)
	case ae == nil && (re == nil || re.HTTPStatusCode() == 0): // no answer at all: refused or reset connection, DNS
		return classify(ErrUnavailable, "network", err)
	}
	return classify(errFailed, code, err)
}
