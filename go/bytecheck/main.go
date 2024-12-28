package main

import (
	"fmt"
	"strconv"
)

func main() {
	s := `{"latitude":-27,"longitude":260}`
	b := []byte(s)
	l := len(b)
	fmt.Printf("[DEBUG]data is [%s]\n", string(b))
	fmt.Printf("[DEBUG]length is [%d]\n", len(b))
	for i := 14; i < len(b); i++ {
		fmt.Printf("[DEBUG]data[%d] is [%s]\n", i, string(b[i]))
	}
	fmt.Printf("[DEBUG]latitude is [%s]\n", string(b[2:10]))
	fmt.Printf("[DEBUG]13 is [%s] %v\n", string(b[13]), b[13])
	fmt.Printf("[DEBUG]14 is [%s] %v\n", string(b[14]), b[14])
	fmt.Printf("[DEBUG]15 is [%s] %v\n", string(b[15]), b[15])
	fmt.Printf("[DEBUG]posComma is [%d]\n", posComma(b))
	p := posComma(b)
	lat, _ := strconv.Atoi(string(b[12:p]))
	lon, _ := strconv.Atoi(string(b[p+13 : l-1]))
	fmt.Printf("[DEBUG]lat is [%d]\n", lat)
	fmt.Printf("[DEBUG]lon is [%d]\n", lon)
	fmt.Printf("[DEBUG]minus is [%v]\n", []byte("-"))
}

func posComma(b []byte) int {
	if b[13] == 44 {
		return 13
	}
	if b[14] == 44 {
		return 14
	}
	if b[15] == 44 {
		return 15
	}
	return -1
}
