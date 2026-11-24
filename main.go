package main

import (
	"log"

	"github.com/vanorro1997-droid/IGoGo/internal/app"
)

// Сохраняем запуск через go run .; основная точка входа находится в cmd/api.
func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
