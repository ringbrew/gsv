package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ringbrew/gsv/cli"
	"github.com/ringbrew/gsv/logger"
	"github.com/ringbrew/gsv/service"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

type captureLogger struct {
	logger.MockLogger
	mu      sync.Mutex
	entries []captured
}

type captured struct {
	level   logger.Level
	msg     string
	traceId string
	extra   map[string]interface{}
}

func (c *captureLogger) add(level logger.Level, e *logger.LogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, captured{level: level, msg: e.Message, traceId: e.TraceId, extra: e.RedactedExtra()})
}

func (c *captureLogger) Debug(e *logger.LogEntry) { c.add(logger.LevelDebug, e) }
func (c *captureLogger) Info(e *logger.LogEntry)  { c.add(logger.LevelInfo, e) }
func (c *captureLogger) Warn(e *logger.LogEntry)  { c.add(logger.LevelWarn, e) }
func (c *captureLogger) Error(e *logger.LogEntry) { c.add(logger.LevelError, e) }

func (c *captureLogger) find(t *testing.T, key string, value interface{}) captured {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.entries {
		if e.extra[key] == value {
			return e
		}
	}
	t.Fatalf("no log entry with extra.%s=%v in %+v", key, value, c.entries)
	return captured{}
}

func useCaptureLogger(t *testing.T) *captureLogger {
	c := &captureLogger{}
	logger.SetLogger(c)
	t.Cleanup(func() { logger.SetLogger(logger.NewDefaultLogger()) })
	return c
}

func TestGrpcServerLogHasCaller(t *testing.T) {
	logs := useCaptureLogger(t)
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	defer otel.SetTracerProvider(prevTP)

	streamTraced := make(chan bool, 1)
	checkStreamCtx := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		_, ok := service.FromContext(ss.Context())
		streamTraced <- ok && service.ServerNameFromContext(ss.Context()) == "configsvc"
		return nil
	}

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(serverNameUnaryInterceptor("configsvc"), TraceUnaryInterceptor(), LogUnaryInterceptor()),
		grpc.ChainStreamInterceptor(serverNameStreamInterceptor("configsvc"), TraceStreamServerInterceptor(), checkStreamCtx),
	)
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go srv.Serve(lis)
	defer srv.Stop()

	opt := cli.Classic("iamsvc")
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(opt.UnaryInterceptors...),
		grpc.WithChainStreamInterceptor(opt.StreamInterceptors...),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)
	if _, err := client.Check(context.Background(), &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}

	e := logs.find(t, "method", "/grpc.health.v1.Health/Check")
	if e.msg != "success" || e.level != logger.LevelInfo {
		t.Errorf("unexpected log %+v", e)
	}
	if e.extra["caller_service"] != "iamsvc" {
		t.Errorf("caller_service = %v", e.extra["caller_service"])
	}
	if e.extra["name"] != "configsvc" {
		t.Errorf("name = %v", e.extra["name"])
	}
	if p, _ := e.extra["peer"].(string); p == "" {
		t.Error("peer should be set")
	}
	if e.traceId == "" {
		t.Error("trace_id should be set")
	}

	stream, err := client.Watch(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = stream.Recv()
	if !<-streamTraced {
		t.Error("stream handler context should carry trace and server name")
	}
}

func TestCallerOverrideAndForwardedMetadata(t *testing.T) {
	logs := useCaptureLogger(t)
	cli.SetDefaultName("default-svc")
	defer cli.SetDefaultName("")

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(TraceUnaryInterceptor(), LogUnaryInterceptor()))
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go srv.Serve(lis)
	defer srv.Stop()

	dial := func(unary ...grpc.UnaryClientInterceptor) healthpb.HealthClient {
		conn, err := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainUnaryInterceptor(unary...),
		)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		return healthpb.NewHealthClient(conn)
	}

	// Classic() without a name falls back to SetDefaultName
	_, _ = dial(cli.Classic().UnaryInterceptors...).Check(context.Background(), &healthpb.HealthCheckRequest{Service: ""})
	logs.find(t, "caller_service", "default-svc")

	// Option.Name is appended last by NewClient and must win
	unary := append(cli.Classic().UnaryInterceptors, cli.CallerUnaryInterceptor("named-svc"))
	_, _ = dial(unary...).Check(context.Background(), &healthpb.HealthCheckRequest{})
	logs.find(t, "caller_service", "named-svc")

	// a service forwarding its incoming metadata must report itself, not its upstream
	forwarded := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(service.MetaKeyCaller, "upstream-svc"))
	_, _ = dial(cli.Classic("middle-svc").UnaryInterceptors...).Check(forwarded, &healthpb.HealthCheckRequest{})
	logs.find(t, "caller_service", "middle-svc")
	for _, e := range logs.entries {
		if e.extra["caller_service"] == "upstream-svc" {
			t.Error("forwarded caller must be overwritten")
		}
	}
}

func TestHttpLoggerFields(t *testing.T) {
	logs := useCaptureLogger(t)

	hl := NewHttpLogger()
	hl.SetName("websvc")
	body := "username=bob&password=hunter2hunter2"
	req := httptest.NewRequest(http.MethodPost, "/login?page=1&access_token=abcdefghijklmn", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer abcdefghijklmnop")
	req.Header.Set("Cookie", "sid=should-not-appear")
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	req.Header.Set(service.HeaderCaller, "gateway")

	rw := NewResponseWriter(httptest.NewRecorder())
	hl.ServeHTTP(rw, req, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	e := logs.find(t, "path", "/login")
	if e.extra["client_ip"] != "203.0.113.9" {
		t.Errorf("client_ip = %v", e.extra["client_ip"])
	}
	if e.extra["caller_service"] != "gateway" {
		t.Errorf("caller_service = %v", e.extra["caller_service"])
	}
	if e.extra["query"] != "page=1&access_token=abc******mn" {
		t.Errorf("query = %v", e.extra["query"])
	}
	if e.extra["body"] != "username=bob&password=hun******r2" {
		t.Errorf("body = %v", e.extra["body"])
	}

	headerStr, ok := e.extra["header"].(string)
	if !ok {
		t.Fatalf("header must stay a JSON string, got %T", e.extra["header"])
	}
	var header map[string][]string
	if err := json.Unmarshal([]byte(headerStr), &header); err != nil {
		t.Fatal(err)
	}
	if _, ok := header["Cookie"]; ok {
		t.Error("Cookie must be dropped")
	}
	if got := header["Authorization"]; len(got) != 1 || got[0] != "Bea******op" {
		t.Errorf("Authorization = %v", got)
	}
	if req.Header.Get("Cookie") == "" {
		t.Error("request header must not be modified")
	}
}
