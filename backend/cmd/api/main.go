package main

import (
	"flag"
	"fmt"
	"ingeneriya/backend/internal/app"
	"ingeneriya/backend/internal/config"
	"log"
)

func main() {
	check := flag.Bool("check-config", false, "Validate configuration without starting HTTP or background workers")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected arguments")
	}
	if *check {
		if _, err := config.Load(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("configuration valid")
		return
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
