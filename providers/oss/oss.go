package oss

import (
	"bytes"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-acme/lego/v4/challenge/http01"
	"log"
	"strings"
)

type HTTPProvider struct {
	bucket    string
	ossClient *oss.Client
}

// NewHTTPProvider returns a HTTPProvider instance with a configured oss bucket and aws session.
// Credentials must be passed in the environment variables.
func NewHTTPProvider(bucket string, ossClient *oss.Client) (*HTTPProvider, error) {
	return &HTTPProvider{
		bucket:    bucket,
		ossClient: ossClient,
	}, nil
}

// Present makes the token available at `HTTP01ChallengePath(token)` by creating a file in the given oss bucket.
func (s *HTTPProvider) Present(domain, token, keyAuth string) error {
	bucket, err := s.ossClient.Bucket(s.bucket)
	if err != nil {
		return fmt.Errorf("oss: bucket异常: %w", err)
	}

	// 设置访问权限为只读
	acl := oss.ObjectACL(oss.ACLPublicRead)
	// 上传验证文件到oss存储bucket
	objectKey := strings.Trim(http01.ChallengePath(token), "/")
	log.Printf("上传HTTP域名(%s)验证文件: key -> %s \n", domain, objectKey)
	err = bucket.PutObject(objectKey, bytes.NewReader([]byte(keyAuth)), acl)
	if err != nil {
		return fmt.Errorf("oss: failed to upload token to oss: %w", err)
	}

	return nil
}

// CleanUp removes the file created for the challenge.
func (s *HTTPProvider) CleanUp(domain, token, keyAuth string) error {
	bucket, err := s.ossClient.Bucket(s.bucket)
	if err != nil {
		return fmt.Errorf("oss: bucket异常: %w", err)
	}

	objectKey := strings.Trim(http01.ChallengePath(token), "/")
	log.Printf("删除HTTP域名(%s)验证文件: key -> %s \n", domain, objectKey)
	err = bucket.DeleteObject(objectKey)
	if err != nil {
		return fmt.Errorf("oss: could not remove file in oss bucket after HTTP challenge: %w", err)
	}

	return nil
}
