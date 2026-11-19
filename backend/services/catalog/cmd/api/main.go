// Kept as a compatible entrypoint for the existing service unit.
package main

import (
	"ingeneriya/backend/internal/app"
	"log"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
