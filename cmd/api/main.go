package main

import (
	"log"

	"github.com/vanorro1997-droid/IGoGo/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
