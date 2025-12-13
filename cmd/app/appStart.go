// Package app is an entry-point into delayed-notifier
package app

import (
	"context"
	"log"
	"time"

	"github.com/UnendingLoop/delayed-notifier/internal/cache"
	"github.com/UnendingLoop/delayed-notifier/internal/queue"
	"github.com/UnendingLoop/delayed-notifier/internal/repository"
	"github.com/UnendingLoop/delayed-notifier/internal/sender"
	"github.com/UnendingLoop/delayed-notifier/internal/service"
	"github.com/UnendingLoop/delayed-notifier/internal/transport/handler"
	"github.com/UnendingLoop/delayed-notifier/internal/worker"

	"github.com/wb-go/wbf/config"
	"github.com/wb-go/wbf/dbpg"
	"github.com/wb-go/wbf/ginext"
	"github.com/wb-go/wbf/redis"
)

func StartApp() {
	// Initializing config from env
	appConfig := config.New()
	appConfig.EnableEnv("")
	if err := appConfig.LoadEnvFiles("./.env"); err != nil {
		log.Fatalf("Failed to load envs: %s\nExiting app...", err)
	}

	// Connecting to Postgres
	dbOptions := dbpg.Options{
		MaxOpenConns:    5,
		MaxIdleConns:    5,
		ConnMaxLifetime: 10 * time.Minute,
	}
	dsnLink := appConfig.GetString("POSTGRES_DSN")
	dbConn, err := dbpg.New(dsnLink, nil, &dbOptions)
	if err != nil {
		log.Fatalf("Failed to connect to PGDB: %s\nExiting app...", err)
		return
	}

	// Initializing Repository
	var repo repository.NotificationRepository
	storeType := appConfig.GetString("STORAGE_TYPE")
	switch storeType {
	case "in-memory":
		repo = repository.NewInMemoryRepo()
	case "postgres":
		repo = repository.NewPostgresRepo(dbConn)
		defer func() {
			if err := dbConn.Master.Close(); err != nil {
				log.Println("Failed to close DB-conn correctly:", err)
			}
		}()
	default:
		log.Fatalf("Storage value %q provided in env is incorrect!\nExiting app...", storeType)
	}

	// Connecting to Redis
	redisAddr := appConfig.GetString("REDIS_ADDR")
	redisPwd := appConfig.GetString("REDIS_PASSWORD")
	rawRedis := redis.New(redisAddr, redisPwd, 0)
	defer rawRedis.Close()
	redisClient := cache.NewRedisCache(rawRedis, 1*time.Hour)

	// Initializing RabbitMQ
	time.Sleep(16 * time.Second) // даем контейнеру реббита нормально запуститься
	rabbitAddr := appConfig.GetString("RABBIT_ADDR")
	rabbitConn, rabbitCH, err := queue.NewRabbitInit(rabbitAddr)
	if err != nil {
		log.Fatalln("Faile to initialize rabbit:", err)
	}
	defer rabbitConn.Close()

	// Initializing Service
	svc := service.NewNotificationService(repo, redisClient, rabbitCH)

	// Launching Worker
	s := sender.NewLogSender()
	w := worker.NewWorker(svc, s, rabbitCH)

	// Launching consumer from Rabbit
	if err := w.StartConsuming(context.Background()); err != nil {
		log.Fatalln("Failed to start worker consumer:", err)
	}

	// Setting up server/endpoints/handlers
	server := ginext.New("") // empty - debug mode, release - prod mode
	handlers := handler.NewHandler(svc)
	api := server.Group("/api")
	notify := api.Group("/notify")

	server.GET("/ping", handlers.SimplePinger)
	notify.POST("", handlers.CreateTask)
	notify.GET("/:uid", handlers.GetTask)
	notify.GET("/all", handlers.GetAll)
	notify.DELETE("/:uid", handlers.DeleteTask)
	server.Static("/web", "./internal/web")

	// Server launch
	if err := server.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
