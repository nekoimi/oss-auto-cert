package main

import (
	"fmt"
	"time"
)

func main() {
	//credentials.NewStaticCredentialsProvider("", "", "")
	//_, err := client.NewClient()
	//if err != nil {
	//	panic(err)
	//}
	//fmt.Println("hello")

	for range time.Tick(3 * time.Second) {
		fmt.Println(time.Now())
	}
}
