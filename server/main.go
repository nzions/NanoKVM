package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"NanoKVM-Server/authn"
	"NanoKVM-Server/common"
	"NanoKVM-Server/config"
	"NanoKVM-Server/logger"
	"NanoKVM-Server/middleware"
	"NanoKVM-Server/router"
	"NanoKVM-Server/service/hdmi"
	"NanoKVM-Server/service/vm"
	"NanoKVM-Server/service/vm/jiggler"
	"NanoKVM-Server/utils"

	"github.com/gin-gonic/gin"
	cors "github.com/rs/cors/wrapper/gin"
)

func main() {
	initialize()
	defer dispose()

	run()
}

func initialize() {
	if err := config.EnsurePicoclawInternalToken(); err != nil {
		log.Fatalf("failed to initialize picoclaw internal token: %v", err)
	}

	logger.Init()

	// migrate the 2.5.15 account store, or seed root now so its first-boot
	// password is on the device screen. The one account is root and its
	// password is root's Linux password.
	if config.GetInstance().Authentication != "disable" {
		if err := authn.DefaultStore.Init(); err != nil {
			log.Printf("ERROR: failed to initialize the root login: %v", err)
		}
	}

	// init screen parameters
	_ = common.GetScreen()

	// HDMI state: libkvm publishes every transition, the hub fans it out to
	// the websocket clients and, over a unix socket, to kvm_system. Started
	// before capture is switched so no transition is missed.
	hdmiCtx, stopHdmi := context.WithCancel(context.Background())
	stopHdmiState = stopHdmi
	go hdmi.Default().Run(hdmiCtx, common.GetKvmVision().WaitHDMI)
	go func() {
		if err := hdmi.Default().ServeSocket(hdmiCtx, hdmi.SocketPath); err != nil {
			log.Printf("ERROR: hdmi state socket: %v", err)
		}
	}()

	// init HDMI
	vm.DisableHdmiCapture()
	time.Sleep(10 * time.Millisecond)
	if !utils.IsHdmiDisabled() {
		vm.EnableHdmiCapture()
	}
	vm.SetHdmiViewerCount(0)

	// run mouse jiggler
	jiggler.GetJiggler().Run()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		sig := <-sigChan
		log.Printf("\nReceived signal: %v\n", sig)

		dispose()
		os.Exit(0)
	}()
}

func run() {
	conf := config.GetInstance()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	// same-origin only unless an allowlist is configured
	if origins := conf.Security.CORSOrigins; len(origins) > 0 {
		r.Use(cors.New(cors.Options{
			AllowedOrigins:   origins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: true,
		}))
	}

	router.Init(r)

	httpAddr := utils.ListenAddr(conf.Host, strconv.Itoa(conf.Port.Http))
	loopbackHTTPAddr := utils.ListenAddr("127.0.0.1", strconv.Itoa(conf.Port.Http))
	needsLoopbackHTTP := utils.NeedsDedicatedLoopbackListener(conf.Host)

	// every listener reports here; the first to stop is fatal
	failed := make(chan error, 3)
	serve := func(name string, listen func() error) {
		go func() {
			failed <- fmt.Errorf("%s server stopped: %w", name, listen())
		}()
	}

	if conf.Proto == "https" {
		if err := utils.EnsureCert(conf.Cert.Crt, conf.Cert.Key); err != nil {
			log.Fatalf("prepare https certificate failed: %v", err)
		}

		httpsPortStr := strconv.Itoa(conf.Port.Https)

		serve("https", func() error {
			server := middleware.NewServer(utils.ListenAddr(conf.Host, httpsPortStr), r)
			return server.ListenAndServeTLS(conf.Cert.Crt, conf.Cert.Key)
		})

		if needsLoopbackHTTP {
			serve("loopback http", func() error {
				return middleware.ListenAndServeLoopbackHTTPRedirect(
					loopbackHTTPAddr,
					httpsPortStr,
					r,
					router.LoopbackHTTPAllowedPaths()...,
				)
			})
		}

		// plain http only redirects to https
		serve("http", func() error {
			return middleware.ListenAndServeLoopbackHTTPRedirect(
				httpAddr,
				httpsPortStr,
				r,
				router.LoopbackHTTPAllowedPaths()...,
			)
		})
	} else {
		if needsLoopbackHTTP {
			serve("loopback http", func() error {
				return middleware.NewServer(loopbackHTTPAddr, r).ListenAndServe()
			})
		}

		serve("http", func() error {
			return middleware.NewServer(httpAddr, r).ListenAndServe()
		})
	}

	log.Fatalf("%v", <-failed)
}

// stopHdmiState ends the HDMI hub loop and its socket.
var stopHdmiState = func() {}

func dispose() {
	stopHdmiState()
	common.GetKvmVision().Close()
}
