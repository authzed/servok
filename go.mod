module github.com/REDACTED/code/servok

go 1.16

require (
	github.com/authzed/grpcutil v0.0.0-20210709212005-3a705ca91827
	github.com/envoyproxy/protoc-gen-validate v0.6.1
	github.com/google/go-cmp v0.5.6
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/go-grpc-middleware/providers/zerolog/v2 v2.0.0-rc.2
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.0.0-rc.2
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/jzelinskie/cobrautil v0.0.4
	github.com/jzelinskie/stringz v0.0.1 // indirect
	github.com/prometheus/client_golang v0.9.4
	github.com/rs/zerolog v1.25.0
	github.com/spf13/cobra v1.2.1
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.24.0
	go.opentelemetry.io/otel/exporters/jaeger v1.0.0 // indirect
	google.golang.org/grpc v1.41.0
	google.golang.org/protobuf v1.27.1
)
