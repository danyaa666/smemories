package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config describes an S3-compatible bucket. Endpoint is empty for AWS S3 and set (with PathStyle)
// for MinIO. ponytail: static keys only; M2 (T-023) adds the default credential chain for task roles.
type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle bool
}

// S3 is the Storage backed by an S3-compatible bucket.
type S3 struct {
	c      *s3.Client
	bucket string
}

// NewS3 builds the client; it does not contact the server.
func NewS3(cfg S3Config) *S3 {
	opts := s3.Options{
		Region:       cfg.Region,
		UsePathStyle: cfg.PathStyle,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		// Send checksums only where S3 requires them: older MinIO builds and other S3 clones
		// reject the optional ones.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}
	if cfg.Endpoint != "" {
		opts.BaseEndpoint = aws.String(cfg.Endpoint)
	}
	c := s3.New(opts)
	return &S3{c: c, bucket: cfg.Bucket}
}

// EnsureBucket creates the bucket when it does not exist (tests and first run; production buckets come from IaC).
func (s *S3) EnsureBucket(ctx context.Context) error {
	_, err := s.c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket})
	var own *types.BucketAlreadyOwnedByYou
	if err != nil && !errors.As(err, &own) {
		return err
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key, contentType string, data []byte) error {
	_, err := s.c.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, Body: bytes.NewReader(data), ContentType: &contentType})
	return err
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
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

func (s *S3) Delete(ctx context.Context, keys ...string) error {
	for i := 0; i < len(keys); i += 1000 { // DeleteObjects takes at most 1000 keys
		if err := s.deleteBatch(ctx, keys[i:min(i+1000, len(keys))]); err != nil {
			return err
		}
	}
	return nil
}

func (s *S3) deleteBatch(ctx context.Context, keys []string) error {
	ids := make([]types.ObjectIdentifier, len(keys))
	for i := range keys {
		ids[i] = types.ObjectIdentifier{Key: &keys[i]}
	}
	out, err := s.c.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: &s.bucket, Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)}})
	if err != nil {
		return err
	}
	if len(out.Errors) > 0 { // per-key failures come back with a 200
		return fmt.Errorf("storage: delete %q: %s", aws.ToString(out.Errors[0].Key), aws.ToString(out.Errors[0].Code))
	}
	return nil
}

func (s *S3) DeletePrefix(ctx context.Context, prefix string) error {
	p := s3.NewListObjectsV2Paginator(s.c, &s3.ListObjectsV2Input{Bucket: &s.bucket, Prefix: &prefix})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
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
