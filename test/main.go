package main

import (
	"fmt"
	"one-api/common"
)

func main() {
	str, _ := common.Password2Hash("wxcas!@#")
	fmt.Print(str)
}
