package oss

import (
	"bytes"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/charmbracelet/log"
	"github.com/go-acme/lego/v4/challenge/http01"
	"strings"
)

type HTTPProvider struct {
	bucket    string
	ossClient *oss.Client
}

func NewHTTPProvider(bucket string, ossClient *oss.Client) (*HTTPProvider, error) {
	return &HTTPProvider{
		bucket:    bucket,
		ossClient: ossClient,
	}, nil
}

func (s *HTTPProvider) Present(domain, token, keyAuth string) error {
	bucket, err := s.ossClient.Bucket(s.bucket)
	if err != nil {
		return fmt.Errorf("oss: bucket异常: %w", err)
	}

	// 设置访问权限为只读
	acl := oss.ObjectACL(oss.ACLPublicRead)
	// 上传验证文件到oss存储bucket
	objectKey := strings.Trim(http01.ChallengePath(token), "/")
	log.Infof("上传HTTP域名(%s)验证文件: key -> %s", domain, objectKey)
	err = bucket.PutObject(objectKey, bytes.NewReader([]byte(keyAuth)), acl)
	if err != nil {
		return fmt.Errorf("oss: failed to upload token to oss: %w", err)
	}

	return nil
}

func (s *HTTPProvider) CleanUp(domain, token, keyAuth string) error {
	bucket, err := s.ossClient.Bucket(s.bucket)
	if err != nil {
		return fmt.Errorf("oss: bucket异常: %w", err)
	}

	objectKey := strings.Trim(http01.ChallengePath(token), "/")
	log.Infof("删除HTTP域名(%s)验证文件: key -> %s", domain, objectKey)
	err = bucket.DeleteObject(objectKey)
	if err != nil {
		return fmt.Errorf("oss: could not remove file in oss bucket after HTTP challenge: %w", err)
	}

	return nil
}
