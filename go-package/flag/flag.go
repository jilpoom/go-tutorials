package main

import (
	"flag"
	"fmt"
)

func main() {
	option := flag.String("option", "default", "option description")

	flag.Parse()

	fmt.Println("option:", *option)

}