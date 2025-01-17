// Copyright 2023 Talhuang <talhuang1231@gmail.com>. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package userservice

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/skeleton1231/gotal/internal/user_service/config"
	"github.com/skeleton1231/gotal/internal/user_service/store"

	"github.com/skeleton1231/gotal/internal/pkg/options"
	"github.com/skeleton1231/gotal/internal/pkg/server"
	pbUser "github.com/skeleton1231/gotal/internal/proto/user"
	ssv1 "github.com/skeleton1231/gotal/internal/user_service/service/server"
	"github.com/skeleton1231/gotal/internal/user_service/store/database"
	"github.com/skeleton1231/gotal/pkg/cache"
	"github.com/skeleton1231/gotal/pkg/log"
	"github.com/skeleton1231/gotal/pkg/shutdown"
	posix "github.com/skeleton1231/gotal/pkg/shutdown/managers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

type apiServer struct {
	gs            *shutdown.GracefulShutdown
	redisOptions  *options.RedisOptions
	httpAPIServer *server.APIServer // embedding internal/pkg/server
	gRPCAPIServer *grpcAPIServer    // embedding grpcAPIServer
}

type preparedAPIServer struct {
	*apiServer
}

type ExtraConfig struct {
	Addr         string
	MaxMsgSize   int
	ServerCert   options.GeneratableKeyCert
	mysqlOptions *options.MySQLOptions
}

func NewAPIServer(cfg *config.Config) (*apiServer, error) {
	gs := shutdown.New()
	gs.AddShutdownManager(posix.NewPosixSignalManager())

	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}

	genericServer, err := server.NewCompletedConfig(genericConfig).New()

	if err != nil {
		return nil, err
	}

	extraConfig, err := buildExtraConfig(cfg)
	if err != nil {
		return nil, err
	}

	extraServer, err := newCompletedExtraConfig(extraConfig).New()
	if err != nil {
		return nil, err
	}

	// Finish the apiServer
	server := &apiServer{
		gs:            gs,
		redisOptions:  cfg.RedisOptions,
		httpAPIServer: genericServer,
		gRPCAPIServer: extraServer,
	}

	return server, nil
}

func (s *apiServer) PrepareRun() preparedAPIServer {

	// initialize the router
	initRouter(s.httpAPIServer.Engine)

	// initialize redis
	s.initRedisStore()

	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {

		mysqlStore, _ := database.GetMySQLFactoryOr(nil)
		if mysqlStore != nil {
			_ = mysqlStore.Close()
		}

		s.gRPCAPIServer.Close()
		s.httpAPIServer.Close()

		return nil
	}))

	return preparedAPIServer{s}
}

func (s preparedAPIServer) Run() error {
	// Start GRPC Server
	go s.gRPCAPIServer.Run()

	// start shutdown managers
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}

	// Start Http/Https Server
	return s.httpAPIServer.Run()
}

type completedExtraConfig struct {
	*ExtraConfig
}

func newCompletedExtraConfig(c *ExtraConfig) *completedExtraConfig {
	if c.Addr == "" {
		c.Addr = "127.0.0.1:8081"
	}

	return &completedExtraConfig{c}
}

// New create a grpcAPIServer instance.
func (c *completedExtraConfig) New() (*grpcAPIServer, error) {
	creds, err := credentials.NewServerTLSFromFile(c.ServerCert.CertKey.CertFile, c.ServerCert.CertKey.KeyFile)
	if err != nil {
		log.Fatalf("Failed to generate credentials %s", err.Error())
	}
	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(c.MaxMsgSize), grpc.Creds(creds)}
	grpcServer := grpc.NewServer(opts...)

	storeIns, _ := database.GetMySQLFactoryOr(c.mysqlOptions)
	logrus.Debugf("Store Connections %v:", storeIns)

	store.SetClient(storeIns)

	userService, _ := ssv1.GetUserInsOr(storeIns)
	// Register GRPC Server
	pbUser.RegisterUserServiceServer(grpcServer, userService)
	reflection.Register(grpcServer)

	return &grpcAPIServer{grpcServer, c.Addr}, nil
}

func buildGenericConfig(cfg *config.Config) (genericConfig *server.Config, lastErr error) {
	genericConfig = server.NewConfig()
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.FeatureOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.SecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.InsecureServing.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	if lastErr = cfg.RateLimitOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	return
}

// nolint: unparam
func buildExtraConfig(cfg *config.Config) (*ExtraConfig, error) {
	return &ExtraConfig{
		Addr:         fmt.Sprintf("%s:%d", cfg.GRPCOptions.BindAddress, cfg.GRPCOptions.BindPort),
		MaxMsgSize:   cfg.GRPCOptions.MaxMsgSize,
		ServerCert:   cfg.SecureServing.ServerCert,
		mysqlOptions: cfg.MySQLOptions,
	}, nil
}

func (s *apiServer) initRedisStore() {
	ctx, cancel := context.WithCancel(context.Background())

	s.gs.AddShutdownCallback(shutdown.ShutdownFunc(func(string) error {
		cancel()

		return nil
	}))

	config := &cache.Config{
		Host:                  s.redisOptions.Host,
		Port:                  s.redisOptions.Port,
		Addrs:                 s.redisOptions.Addrs,
		MasterName:            s.redisOptions.MasterName,
		Username:              s.redisOptions.Username,
		Password:              s.redisOptions.Password,
		Database:              s.redisOptions.Database,
		MaxIdle:               s.redisOptions.MaxIdle,
		MaxActive:             s.redisOptions.MaxActive,
		Timeout:               s.redisOptions.Timeout,
		EnableCluster:         s.redisOptions.EnableCluster,
		UseSSL:                s.redisOptions.UseSSL,
		SSLInsecureSkipVerify: s.redisOptions.SSLInsecureSkipVerify,
	}

	// try to connect to redis
	go cache.ConnectToRedisV2(ctx, config)
}
