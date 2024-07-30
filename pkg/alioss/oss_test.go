package alioss

import (
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/nekoimi/oss-auto-cert/config"
	"os"
	"testing"
)

func TestBucketService_UpgradeCert(t *testing.T) {
	credentialsProvider, err := oss.NewEnvironmentVariableCredentialsProvider()
	if err != nil {
		t.Fatalf("缺少OSS访问AccessKey环境变量配置: %s", err.Error())
	}

	access := credentialsProvider.GetCredentials()

	name := os.Getenv("TEST_NAME")
	endpoint := os.Getenv("TEST_ENDPOINT")
	domain := os.Getenv("TEST_DOMAIN")
	certID := os.Getenv("TEST_CERT_ID")

	b, err := New(config.Bucket{
		Name:     name,
		Endpoint: endpoint,
	}, access)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = b.UpgradeCert(domain, certID)
	if err != nil {
		t.Fatal(err.Error())
	}
}
