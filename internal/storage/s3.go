package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Defaults for the S3Config timeouts and retries. The AWS client default is about 25 s to give up on a refused connection and
// three silent attempts on top, which makes a storage outage cost a request half a minute (T-075).
const (
	DefaultDialTimeout           = 3 * time.Second
	DefaultTLSHandshakeTimeout   = 5 * time.Second
	DefaultResponseHeaderTimeout = 15 * time.Second
	DefaultOperationTimeout      = 60 * time.Second // one call end to end, body included (a 10 MiB photo on a slow link)
	DefaultMaxAttempts           = 3                // first try included
)

// S3Config describes an S3-compatible bucket. Endpoint is empty for AWS S3 and set (with PathStyle)
// for a local store. With AccessKey empty the client uses the AWS default credential chain (environment, the ECS task role, EC2 instance
// role, shared config), which is how production runs; static keys are for the local store in dev and test.
//
// A zero timeout or MaxAttempts takes the Default* value above. S3Config is passed by value on purpose: start-up settings, read once.
type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle bool

	// Credentials overrides the credential source (tests); it wins over AccessKey and the default chain.
	Credentials aws.CredentialsProvider

	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	OperationTimeout      time.Duration
	MaxAttempts           int
}

// S3 is the Storage backed by an S3-compatible bucket.
type S3 struct {
	c       *s3.Client
	bucket  string
	opLimit time.Duration
}

// orDefault returns v, or def when v is zero.
func orDefault[T ~int | ~int64](v, def T) T {
	if v == 0 {
		return def
	}
	return v
}

// NewS3 builds the client; it does not contact the server. The default credential chain is resolved lazily on the first call
// (and refreshed by the SDK before it expires), so a missing role shows up as an error of that call, not here. The error
// return is a broken shared AWS config file or environment.
func NewS3(cfg S3Config) (*S3, error) {
	creds := cfg.Credentials
	switch {
	case creds != nil:
	case cfg.AccessKey != "":
		creds = credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	default:
		// LoadDefaultConfig builds the provider chain here without network calls; the metadata and container endpoints are asked on first use.
		dc, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cfg.Region))
		if err != nil {
			return nil, apperr.Wrap(apperr.Internal, err, "storage: AWS default credential chain")
		}
		creds = dc.Credentials
	}
	httpc := awshttp.NewBuildableClient().
		WithDialerOptions(func(d *net.Dialer) { d.Timeout = orDefault(cfg.DialTimeout, DefaultDialTimeout) }).
		WithTransportOptions(func(t *http.Transport) {
			t.TLSHandshakeTimeout = orDefault(cfg.TLSHandshakeTimeout, DefaultTLSHandshakeTimeout)
			t.ResponseHeaderTimeout = orDefault(cfg.ResponseHeaderTimeout, DefaultResponseHeaderTimeout)
		})
	opts := s3.Options{
		Region:           cfg.Region,
		UsePathStyle:     cfg.PathStyle,
		Credentials:      creds,
		HTTPClient:       httpc,
		RetryMaxAttempts: orDefault(cfg.MaxAttempts, DefaultMaxAttempts),
		// Send checksums only where S3 requires them: older S3 servers and other S3 clones
		// reject the optional ones.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}
	if cfg.Endpoint != "" {
		opts.BaseEndpoint = aws.String(cfg.Endpoint)
	}
	c := s3.New(opts)
	return &S3{c: c, bucket: cfg.Bucket, opLimit: orDefault(cfg.OperationTimeout, DefaultOperationTimeout)}, nil
}

// op bounds one call (retries included) by the operation timeout.
func (s *S3) op(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.opLimit)
}

// EnsureBucket creates the bucket when it does not exist (tests and first run; production buckets come from IaC).
func (s *S3) EnsureBucket(ctx context.Context) error {
	ctx, cancel := s.op(ctx)
	defer cancel()
	_, err := s.c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket})
	var own *types.BucketAlreadyOwnedByYou
	if err != nil && !errors.As(err, &own) {
		return err
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key, contentType string, data []byte) error {
	ctx, cancel := s.op(ctx)
	defer cancel()
	_, err := s.c.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, Body: bytes.NewReader(data), ContentType: &contentType})
	return err
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	ctx, cancel := s.op(ctx) // covers reading the body too
	defer cancel()
	out, err := s.c.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		var nf *types.NoSuchKey
		if errors.As(err, &nf) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()
	return io.ReadAll(out.Body)
}

// notFound reports whether err is the store saying the key does not exist (HEAD answers NotFound, GET NoSuchKey).
func notFound(err error) bool {
	var nf *types.NotFound
	var nk *types.NoSuchKey
	return errors.As(err, &nf) || errors.As(err, &nk)
}

// Open sizes the object with a HEAD and then reads it lazily: the first Read (and the first after a Seek) is a GET for
// "bytes=<position>-", so HEAD requests and 304 answers never fetch a body and a download holds no more than the
// copy buffer in memory. The body is read under ctx (the request's), not under the operation timeout.
func (s *S3) Open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	hctx, cancel := s.op(ctx)
	defer cancel()
	out, err := s.c.HeadObject(hctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		if notFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s3Object{s: s, ctx: ctx, key: key, size: aws.ToInt64(out.ContentLength)}, nil
}

// s3Object is an object of the bucket read through ranged GETs. Not safe for concurrent use.
type s3Object struct {
	s    *S3
	ctx  context.Context
	key  string
	size int64
	pos  int64         // where the next Read starts
	body io.ReadCloser // the open GET, positioned at pos; nil until needed
}

func (o *s3Object) Read(p []byte) (int, error) {
	if o.pos >= o.size {
		return 0, io.EOF
	}
	if o.body == nil {
		out, err := o.s.c.GetObject(o.ctx, &s3.GetObjectInput{Bucket: &o.s.bucket, Key: &o.key, Range: aws.String(fmt.Sprintf("bytes=%d-", o.pos))})
		if err != nil {
			if notFound(err) {
				return 0, ErrNotFound
			}
			return 0, err
		}
		o.body = out.Body
	}
	n, err := o.body.Read(p)
	o.pos += int64(n)
	if errors.Is(err, io.EOF) && o.pos < o.size {
		err = io.ErrUnexpectedEOF // the store ended the body early: never present it as a complete object
	}
	return n, err
}

func (o *s3Object) Seek(off int64, whence int) (int64, error) {
	var np int64
	switch whence {
	case io.SeekStart:
		np = off
	case io.SeekCurrent:
		np = o.pos + off
	case io.SeekEnd:
		np = o.size + off
	default:
		return 0, apperr.New(apperr.Internal, "storage: bad seek whence")
	}
	if np < 0 {
		return 0, apperr.New(apperr.Internal, "storage: negative seek position")
	}
	if np != o.pos {
		_ = o.Close() // the open GET is at the old position
		o.pos = np
	}
	return np, nil
}

// Close ends the GET in progress, if any; the object can still be read afterwards (the next Read opens a new GET).
func (o *s3Object) Close() error {
	if o.body == nil {
		return nil
	}
	err := o.body.Close()
	o.body = nil
	return err
}

// deleteBatchMax is the most keys one DeleteObjects call takes.
const deleteBatchMax = 1000

func (s *S3) Delete(ctx context.Context, keys ...string) error {
	for i := 0; i < len(keys); i += deleteBatchMax {
		if err := s.deleteBatch(ctx, keys[i:min(i+deleteBatchMax, len(keys))]); err != nil {
			return err
		}
	}
	return nil
}

func (s *S3) deleteBatch(ctx context.Context, keys []string) error {
	ctx, cancel := s.op(ctx)
	defer cancel()
	ids := make([]types.ObjectIdentifier, len(keys))
	for i := range keys {
		ids[i] = types.ObjectIdentifier{Key: &keys[i]}
	}
	out, err := s.c.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: &s.bucket, Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)}})
	if err != nil {
		return err
	}
	if len(out.Errors) > 0 { // per-key failures come back with a 200
		return apperr.New(apperr.Internal, fmt.Sprintf("storage: delete %q: %s", aws.ToString(out.Errors[0].Key), aws.ToString(out.Errors[0].Code)))
	}
	return nil
}

func (s *S3) DeletePrefix(ctx context.Context, prefix string) error {
	p := s3.NewListObjectsV2Paginator(s.c, &s3.ListObjectsV2Input{Bucket: &s.bucket, Prefix: &prefix})
	for p.HasMorePages() {
		pctx, cancel := s.op(ctx)
		page, err := p.NextPage(pctx)
		cancel()
		if err != nil {
			return err
		}
		keys := make([]string, len(page.Contents))
		for i, o := range page.Contents {
			keys[i] = aws.ToString(o.Key)
		}
		if len(keys) > 0 {
			if err := s.Delete(ctx, keys...); err != nil {
				return err
			}
		}
	}
	return nil
}
