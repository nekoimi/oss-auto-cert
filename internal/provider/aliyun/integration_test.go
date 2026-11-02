//go:build integration

package aliyun

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nekoimi/oss-auto-cert/internal/config"
)

// 此测试只读专用测试 Bucket，必须显式提供测试资源并启用 integration tag。
func TestIntegrationDiscover(t *testing.T) {
	name, endpoint := os.Getenv("TEST_NAME"), os.Getenv("TEST_ENDPOINT")
	if name == "" || endpoint == "" {
		t.Skip("需要 TEST_NAME / TEST_ENDPOINT 专用测试资源")
	}
	cloud, err := New(os.Getenv("OSS_ACCESS_KEY_ID"), os.Getenv("OSS_ACCESS_KEY_SECRET"), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := cloud.Discover(ctx, config.Bucket{Name: name, Endpoint: endpoint}); err != nil {
		t.Fatal(err)
	}
}
