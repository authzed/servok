package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/authzed/grpcutil"
	grpcmw "github.com/grpc-ecosystem/go-grpc-middleware"
	grpczerolog "github.com/grpc-ecosystem/go-grpc-middleware/providers/zerolog/v2"
	grpclog "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpcprom "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/jzelinskie/cobrautil"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	v1 "github.com/REDACTED/code/servok/internal/proto/servok/api/v1"
	"github.com/REDACTED/code/servok/internal/services"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "servok",
		Short: "Serve endpoint metadata for client side load balancing.",
		PreRunE: cobrautil.CommandStack(
			cobrautil.SyncViperPreRunE("servok"),
			cobrautil.ZeroLogPreRunE,
		),
		Run: rootRun,
	}

	rootCmd.Flags().String("grpc-addr", ":50051", "address to listen on for serving gRPC services")
	rootCmd.Flags().String("grpc-cert-path", "", "local path to the TLS certificate used to serve gRPC services")
	rootCmd.Flags().String("grpc-key-path", "", "local path to the TLS key used to serve gRPC services")
	rootCmd.Flags().Bool("grpc-no-tls", false, "serve unencrypted gRPC services")
	rootCmd.Flags().String("metrics-addr", ":9090", "address to listen on for serving metrics and profiles")

	cobrautil.RegisterZeroLogFlags(rootCmd.Flags())

	rootCmd.Execute()
}

func rootRun(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())

	var sharedOptions []grpc.ServerOption
	sharedOptions = append(sharedOptions, grpcmw.WithUnaryServerChain(
		otelgrpc.UnaryServerInterceptor(),
		grpcprom.UnaryServerInterceptor,
		grpclog.UnaryServerInterceptor(grpczerolog.InterceptorLogger(log.Logger)),
	))

	var grpcServer *grpc.Server
	if cobrautil.MustGetBool(cmd, "grpc-no-tls") {
		grpcServer = grpc.NewServer(sharedOptions...)
	} else {
		var err error
		grpcServer, err = NewTlsGrpcServer(
			cobrautil.MustGetStringExpanded(cmd, "grpc-cert-path"),
			cobrautil.MustGetStringExpanded(cmd, "grpc-key-path"),
			sharedOptions...,
		)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to create TLS gRPC server")
		}
	}

	healthSrv := grpcutil.NewAuthlessHealthServer()

	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus(
		v1.EndpointService_ServiceDesc.ServiceName,
		healthpb.HealthCheckResponse_SERVING,
	)

	servicer, err := services.NewEndpointServicer(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("unable to initialize servicer")
	}
	v1.RegisterEndpointServiceServer(grpcServer, servicer)
	reflection.Register(grpcServer)

	go func() {
		addr := cobrautil.MustGetString(cmd, "grpc-addr")
		l, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatal().Str("addr", addr).Msg("failed to listen on addr for gRPC server")
		}

		log.Info().Str("addr", addr).Msg("gRPC server started listening")
		grpcServer.Serve(l)
	}()

	metricsrv := NewMetricsServer(cobrautil.MustGetString(cmd, "metrics-addr"))
	go func() {
		if err := metricsrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("failed while serving metrics")
		}
	}()

	signalctx, _ := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	<-signalctx.Done()

	log.Info().Msg("shutting down")
	cancel()
	grpcServer.GracefulStop()

	if err := metricsrv.Close(); err != nil {
		log.Fatal().Err(err).Msg("failed while shutting down metrics server")
	}
}

func NewMetricsServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}

func NewTlsGrpcServer(certPath, keyPath string, opts ...grpc.ServerOption) (*grpc.Server, error) {
	if certPath == "" || keyPath == "" {
		return nil, errors.New("missing one of required values: cert path, key path")
	}

	creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	opts = append(opts, grpc.Creds(creds))
	return grpc.NewServer(opts...), nil
}
