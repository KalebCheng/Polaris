package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Store S3 / MinIO 等兼容对象存储
type S3Store struct {
	cfg    Config
	client *s3.Client
}

func newS3Store(cfg Config) (*S3Store, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if cfg.AccessKey != "" {
		options = append(options, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}
	loaded, err := awsconfig.LoadDefaultConfig(context.Background(), options...)
	if err != nil {
		return nil, fmt.Errorf("加载对象存储配置失败: %w", err)
	}
	client := s3.NewFromConfig(loaded, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.PathStyle
	})
	return &S3Store{cfg: cfg, client: client}, nil
}

// Kind 后端类型
func (s *S3Store) Kind() string { return "s3" }

// Put 上传对象并返回访问地址
func (s *S3Store) Put(ctx context.Context, obj Object) (string, error) {
	key := withPrefix(s.cfg.PathPrefix, obj.Key)
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(key),
		Body:   obj.Body,
	}
	if obj.ContentType != "" {
		input.ContentType = aws.String(obj.ContentType)
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return "", fmt.Errorf("上传对象失败: %w", err)
	}
	return s.PublicURL(key), nil
}

// PublicURL 对象访问地址（优先自定义域名）
func (s *S3Store) PublicURL(key string) string {
	if s.cfg.Domain != "" {
		return strings.TrimSuffix(s.cfg.Domain, "/") + "/" + key
	}
	endpoint := strings.TrimSuffix(s.cfg.Endpoint, "/")
	if endpoint == "" {
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.cfg.Bucket, orRegion(s.cfg.Region), key)
	}
	if s.cfg.PathStyle {
		return endpoint + "/" + s.cfg.Bucket + "/" + key
	}
	host := strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	scheme := "https://"
	if strings.HasPrefix(endpoint, "http://") {
		scheme = "http://"
	}
	return fmt.Sprintf("%s%s.%s/%s", scheme, s.cfg.Bucket, host, key)
}

// Delete 删除对象
func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(withPrefix(s.cfg.PathPrefix, key)),
	})
	return err
}

// Probe 连通性自检：写入并删除探测对象
func (s *S3Store) Probe(ctx context.Context) error {
	key := fmt.Sprintf(".probe-%d", time.Now().UnixNano())
	if _, err := s.Put(ctx, Object{Key: key, Body: strings.NewReader("ok"), ContentType: "text/plain", Size: 2}); err != nil {
		return err
	}
	return s.Delete(ctx, key)
}

func orRegion(region string) string {
	if region == "" {
		return "us-east-1"
	}
	return region
}
