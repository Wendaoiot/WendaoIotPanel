// Command wendao-broker 是独立部署的 MQTT 接入服务（mochi-mqtt，MIT）。
//
// 与 Go 后端（wendao-server）、MySQL 同为独立服务，通过 compose 内网协作：
//   - 设备 MQTT TCP/TLS 接入（1883/8883），认证/ACL/互踢在本进程完成；
//   - 认证只读 MySQL 中的设备一机一密/产品一型一密；
//   - 设备上下线经内部 HTTP（X-Sync-Key）上报后端落库并广播前端；
//   - 后端禁用/重置激活/业务互踢时经内部 HTTP /internal/v1/kick 调本进程踢线。
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"wendaoiotpanel/internal/broker"
	"wendaoiotpanel/internal/config"
	"wendaoiotpanel/internal/internalhttp"
	"wendaoiotpanel/internal/store"
)

func main() {
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.ValidateBroker(); err != nil {
		log.Fatalf("config invalid: %v", err)
	}
	log.Printf("broker config loaded: mysql=%s:%d/%s mqtt tcp=:%d tls=:%d internal=%s",
		cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.Database,
		cfg.Broker.ListenPort, cfg.Broker.TLSPort, cfg.Broker.InternalAddr)

	// broker 仅对 MySQL 做认证只读访问；表结构由后端 AutoMigrate 负责。
	s, err := store.New(cfg.MySQLDSN())
	if err != nil {
		log.Fatalf("connect mysql: %v", err)
	}

	// 设备上下线 → 内部 HTTP 上报后端。
	sink := internalhttp.NewStatusSink(cfg.Sync.ServerURL, cfg.Sync.Secret)

	b, err := broker.New(broker.Config{
		ListenPort:      cfg.Broker.ListenPort,
		TLSPort:         cfg.Broker.TLSPort,
		TLSCert:         cfg.Broker.TLSCert,
		TLSKey:          cfg.Broker.TLSKey,
		PersistencePath: cfg.Broker.PersistencePath,
		ServerUsername:  cfg.Broker.ServerUsername,
		ServerPassword:  cfg.Broker.ServerPassword,
	}, s, sink)
	if err != nil {
		log.Fatalf("create broker: %v", err)
	}
	if err := b.Start(); err != nil {
		log.Fatalf("start broker: %v", err)
	}

	// 内部 HTTP：健康探针 + 后端踢线入口（仅内网，X-Sync-Key 鉴权）。
	internal := internalhttp.NewServer(cfg.Broker.InternalAddr, cfg.Sync.Secret, b)
	internal.Start()

	log.Printf("wendao-broker ready: mqtt tcp=:%d tls=:%d (device session kick active)",
		cfg.Broker.ListenPort, cfg.Broker.TLSPort)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down broker ...")
	_ = internal.Close()
	_ = b.Close()
}
