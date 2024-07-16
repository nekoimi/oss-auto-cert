package main

import (
	"fmt"
	"github.com/alibabacloud-go/alibabacloud-gateway-spi/client"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func main() {
	credentials.NewStaticCredentialsProvider("", "", "")
	_, err := client.NewClient()
	if err != nil {
		panic(err)
	}
	fmt.Println("hello")
}
