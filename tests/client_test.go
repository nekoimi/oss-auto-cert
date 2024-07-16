package tests

import (
	"fmt"
	rpc "github.com/alibabacloud-go/tea-rpc/client"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/facebody"
	credential "github.com/aliyun/credentials-go/credentials"
	"testing"
)

func TestSdk(t *testing.T) {
	config := new(rpc.Config)

	// 使用 ak 初始化 config
	config.SetAccessKeyId("ACCESS_KEY_ID").
		SetAccessKeySecret("ACCESS_KEY_SECRET").
		SetRegionId("cn-hangzhou").
		SetEndpoint("facebody.cn-hangzhou.aliyuncs.com")

	// 使用 credential 初始化 config
	credentialConfig := &credential.Config{
		AccessKeyId:     config.AccessKeyId,
		AccessKeySecret: config.AccessKeySecret,
		SecurityToken:   config.SecurityToken,
	}
	// 关于 credenial 的创建可以参考 https://github.com/aliyun/credentials-go/blob/master/README-CN.md
	cred, err := credential.NewCredential(credentialConfig)
	if err != nil {
		panic(err)
	}
	config.SetCredential(cred).
		SetEndpoint("facebody.cn-hangzhou.aliyuncs.com")

	// 创建客户端
	client, err := facebody.NewClient()
	if err != nil {
		panic(err)
	}

	//// 初始化 runtimeObject
	//runtimeObject := new(util.RuntimeOptions).SetAutoretry(false).
	//	SetMaxIdleConns(3)

	// 初始化 request
	request := new(facebody.DetectFaceRequest)

	// 调用 api
	resp, err := client.DetectFace(request)
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(resp)

	//// 使用文件上传
	//uploadRequest := new(facebody.DetectFaceAdvanceRequest).SetImageURLObject(strings.NewReader("demo"))
	//
	//// 调用 api
	//uploadResp, err := client.DetectFaceAdvance(uploadRequest, runtimeObject)
	//if err != nil {
	//	fmt.Println(err.Error())
	//}
	//fmt.Println(uploadResp)
}
