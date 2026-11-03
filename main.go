package main

import (
    "github.com/gin-gonic/gin"
    "github.com/vanorro1997-droid/IGoGo/health"
)

func main() {
    r := gin.Default()
    r.GET("/health", health.Health)
    r.Run(":8080")
}