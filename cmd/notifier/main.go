package main

import (
	"context"
	"log"

	"github.com/wb-go/wbf/config"
	"github.com/wb-go/wbf/ginext"
	"github.com/wb-go/wbf/redis"
)

func main() {
	rediska := redis.New("addr string", "password string", 555)
	rediska.Close()
	appConfig := config.New()
	server := ginext.New("")
	server.GET("/ping", func(ctx *context.Context) (any, error) {
		return map[string]string{"message": "pong"}, nil
	})

	if err := server.Run("8080"); err != nil {
		log.Fatal(err)
	}
}
