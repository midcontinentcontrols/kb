package main

import "log"

func main() {
	if err := buildCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
