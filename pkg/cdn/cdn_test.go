package cdn

import (
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/charmbracelet/log"
	"github.com/nekoimi/oss-auto-cert/pkg/dto"
	"github.com/nekoimi/oss-auto-cert/pkg/utils"
	"os"
	"strconv"
	"testing"
)

func TestService_UpgradeCert(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	credentialsProvider, err := oss.NewEnvironmentVariableCredentialsProvider()
	if err != nil {
		t.Fatalf("缺少OSS访问AccessKey环境变量配置: %s", err.Error())
	}

	access := credentialsProvider.GetCredentials()

	domain := os.Getenv("TEST_DOMAIN")
	certID := os.Getenv("TEST_CERT_ID")

	c := New(access)

	int64Str := utils.SplitFirst(certID, "-")
	int64ID, err := strconv.ParseInt(int64Str, 10, 64)
	if err != nil {
		t.Fatalf(err.Error())
	}

	err = c.UpgradeCert(domain, &dto.CertInfo{
		ID:     int64ID,
		Name:   "",
		Region: "",
		Domain: "",
	})
	if err != nil {
		t.Fatalf(err.Error())
	}
}
