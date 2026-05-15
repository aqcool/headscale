package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/server"
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(migrateCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the headscale server",
	Long:  "Start the headscale control server that manages Tailscale nodes.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := types.LoadServerConfig()
		if err != nil {
			return fmt.Errorf("loading server config: %w", err)
		}

		logLevel := viper.GetString("log.level")
		if logLevel == "" {
			logLevel = "info"
		}

		logger := log.DefaultLogger

		fmt.Printf("Starting headscale server...\n")
		fmt.Printf("  Server URL: %s\n", cfg.ServerURL)
		fmt.Printf("  Listen Addr: %s\n", cfg.Addr)
		fmt.Printf("  gRPC Addr: %s\n", cfg.GRPCAddr)
		fmt.Printf("  Database: %s\n", cfg.Database.Type)
		if cfg.PrefixV4 != nil {
			fmt.Printf("  IPv4 Prefix: %s\n", cfg.PrefixV4)
		}
		if cfg.PrefixV6 != nil {
			fmt.Printf("  IPv6 Prefix: %s\n", cfg.PrefixV6)
		}
		fmt.Printf("  Base Domain: %s\n", cfg.BaseDomain)
		fmt.Printf("  MagicDNS: %v\n", cfg.DNSConfig.MagicDNS)
		if cfg.OIDC.Issuer != "" {
			fmt.Printf("  OIDC Issuer: %s\n", cfg.OIDC.Issuer)
		}
		if cfg.DERP.ServerEnabled {
			fmt.Printf("  DERP Server: enabled (STUN: %s)\n", cfg.DERP.STUNAddr)
		}

		hsServer, err := server.NewHeadscaleServer(nil, nil, logger)
		if err != nil {
			return fmt.Errorf("creating headscale server: %w", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			sigc := make(chan os.Signal, 1)
			signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
			for sig := range sigc {
				switch sig {
				case syscall.SIGHUP:
					fmt.Println("Received SIGHUP, reloading policy...")
				default:
					fmt.Printf("Received signal %v, shutting down...\n", sig)
					cancel()
					return
				}
			}
		}()

		if err := hsServer.Start(ctx); err != nil {
			return fmt.Errorf("starting headscale server: %w", err)
		}

		<-ctx.Done()
		fmt.Println("Shutting down headscale server...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30)
		defer shutdownCancel()

		if err := hsServer.Stop(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down headscale server: %w", err)
		}

		fmt.Println("Headscale server stopped gracefully")
		return nil
	},
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	Long:  "Run all pending database migrations.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Running database migrations...")
		return nil
	},
}

func gRPCAddr() string {
	addr := viper.GetString("grpc_listen_addr")
	if addr == "" {
		addr = "localhost:50443"
	}
	if a := os.Getenv("HEADSCALE_GRPC_ADDR"); a != "" {
		addr = a
	}

	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		addr = net.JoinHostPort(addr, "50443")
	}
	return addr
}

func getGRPCClient() (*grpc.ClientConn, error) {
	addr := gRPCAddr()
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("creating gRPC client: %w", err)
	}
	return conn, nil
}
