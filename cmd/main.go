package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/denizumutdereli/stream-services/internal/config"
	leader "github.com/denizumutdereli/stream-services/internal/etcd"
	"github.com/denizumutdereli/stream-services/internal/factory"
	"github.com/denizumutdereli/stream-services/internal/handler"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"net/http/pprof"
	_ "net/http/pprof"
)

var serviceName string
var port string

func main() {

	config, err := config.GetConfig()
	if err != nil {
		log.Fatal("Fatal error on loading config", err)
	}

	logger := config.Logger

	defaultPort := config.GoServicePort // Default port

	flag.StringVar(&serviceName, "service", "", "Name of the service")
	flag.StringVar(&port, "port", config.GoServicePort, "Port of the service")
	flag.Parse()

	if serviceName == "" {
		logger.Fatal("Please provide a service name",
			zap.Strings("AllowedServices", config.AllowedServices))
	}

	config.ServiceName = serviceName

	if _, err := strconv.Atoi(port); err != nil {
		logger.Warn("Invalid Rest port provided. Using default port",
			zap.String("providedPort", port),
			zap.String("defaultPort", defaultPort))
	} else {
		config.GoServicePort = port
	}

	logger.Info("Starting the service...",
		zap.String("AppName", config.AppName),
		zap.String("Service", serviceName),
		zap.String("RestPort", port))

	logger.Info("---------building packages---------")

	config.IsLeader = make(chan bool, 1)

	ctx := context.Background()

	ele, err := leader.NewLeaderElectionManager(ctx, config)
	if err != nil {
		logger.Fatal("Error initializing Etcd leader election", zap.Error(err))
	}

	if err := ele.SetNodeReadiness(ctx); err != nil {
		logger.Warn("Failed to set node readiness", zap.Error(err))
	}

	nodeCount, err := ele.CountReadyNodes(ctx)
	if err != nil {
		logger.Warn("Failed to count ready nodes", zap.Error(err))
	} else {
		config.EtcdNodes = nodeCount
	}

	config.IsLeader <- false // default
	go func() {
		for {
			isLeader := <-config.IsLeader
			if !isLeader {
				if err := ele.BecomeLeader(ctx); err != nil {
					logger.Warn("Failed to become leader", zap.Error(err))
					config.IsLeader <- false
				} else {
					config.EtcdNodes = 1 // no pending latency *1
					config.IsLeader <- true
					return
				}
			}

			// Refresh the node's readiness
			if err := ele.SetNodeReadiness(ctx); err != nil {
				logger.Warn("Failed to refresh node readiness", zap.Error(err))
			}

			nodeCount, err := ele.CountReadyNodes(ctx)
			if err != nil {
				logger.Warn("Failed to count ready nodes", zap.Error(err))
			} else {
				if nodeCount >= 5 {
					nodeCount = 5
				}

				config.EtcdNodes = nodeCount
			}

			time.Sleep(10 * time.Second)
		}
	}()

	serviceFactory, err := factory.NewServiceFactory(config)
	if err != nil {
		logger.Fatal("Error creating service factory", zap.Error(err))
	}

	streamService, err := serviceFactory.NewStreamService(ctx, serviceName)
	if err != nil {
		logger.Fatal("Error creating stream service", zap.Error(err))
	}

	serviceHandler := handler.NewRestHandler(streamService, config)

	router := gin.Default()

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Welcome to the " + config.AppName})
	})

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"message": "Method or route not found in: " + config.AppName})
	})

	router.GET("/live", serviceHandler.Live)
	router.GET("/read", serviceHandler.Read)

	// Register pprof routes
	router.GET("/debug/pprof/", gin.WrapF(pprof.Index))
	router.GET("/debug/pprof/cmdline", gin.WrapF(pprof.Cmdline))
	router.GET("/debug/pprof/profile", gin.WrapF(pprof.Profile))
	router.GET("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
	router.POST("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
	router.GET("/debug/pprof/trace", gin.WrapF(pprof.Trace))
	router.GET("/debug/pprof/goroutine", gin.WrapH(pprof.Handler("goroutine")))
	router.GET("/debug/pprof/heap", gin.WrapH(pprof.Handler("heap")))
	router.GET("/debug/pprof/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
	router.GET("/debug/pprof/block", gin.WrapH(pprof.Handler("block")))

	srv := &http.Server{
		Addr:    ":" + config.GoServicePort,
		Handler: router,
	}

	go func() {
		logger.Info("REST Server is starting", zap.String("port", config.GoServicePort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Error running REST server", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit

	logger.Info("Server is shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	ele.Close()

	logger.Info("Server exiting")
}
