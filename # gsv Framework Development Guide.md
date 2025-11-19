# gsv Framework Development Guide

## 1. Overview

This document serves as the official guide for developing Go backend services using the `gsv` framework. `gsv` is designed to follow **Clean Architecture** principles and organizes code in a **Domain-Driven** manner.

This guide details the project architecture, directory structure, and core development workflows. It is intended to provide a clear context for both **developers and AI programming assistants** to facilitate daily development tasks.

> **Gsv also provides a command-line toolkit. If you want to use any of Gsv's commands, please make sure you are in the root directory of your project.**

### 1.1 Core Project Architecture

The `gsv` project structure adheres to the separation of concerns, primarily divided into three layers:

*   **Domain Layer**: The core business logic layer. It contains business Entities, Use Cases, and Repository Interfaces. This layer does not depend on any external implementations.
*   **Delivery Layer**: The entry and exit points of the application. It is responsible for dispatching external requests (e.g., HTTP, gRPC) or events (e.g., Cron jobs, Message Queues) to the corresponding Domain Use Cases for processing.
*   **Infrastructure Layer**: Provides implementations for external dependencies required by the project, such as database connections (DAO), Redis clients, and gRPC clients. These instances are provided to the Domain layer via dependency injection.

### 1.2 Directory Structure

A typical `gsv` project directory structure is as follows:

```
📁 my-gsv-project/
├── 📁 build/             # Build-related files (Dockerfile, docker-compose.yml)
├── 📁 cmd/               # Main application entrypoints (main.go), can have multiple
├── 📁 export/            # Definitions to be exposed externally (e.g., generated gRPC code)
├── 📁 internal/          # Core business logic of the project
│   ├── 📁 conf/          # Business configuration definitions (config.go)
│   ├── 📁 delivery/      # Delivery layer implementations (HTTP, gRPC, Cron, MQ)
│   └── 📁 domain/        # Domain layer implementations (business core)
├── 📁 openapi/           # Auto-generated Swagger/OpenAPI JSON files
├── 📁 proto/             # .proto definition files for gRPC
├── 📄 .gitignore
├── 📄 go.mod
├── 📄 go.sum
├── 📄 Makefile
└── 📄 README.md
```

## 2. Domain Layer (`internal/domain`)

The Domain layer is the heart of the business logic, where all business use cases are implemented.

### 2.1 Domain Context (`domain/context.go`)

This file defines the `UseCaseContext` struct, which acts as the application's runtime **Dependency Injection (DI) container**. All infrastructure layer instances (e.g., database, Redis, external service clients) are initialized and managed here.

**Code Example:**
```golang
// internal/domain/context.go
package domain

import (
	// ... imports
	"github.com/ringbrew/gsv/cli"
	"github.com/ringbrew/gsv/discovery"
)

// UseCaseContext holds all shared resources needed by use cases.
type UseCaseContext struct {
	Config    conf.Config
	Signal    context.Context
	cancel    context.CancelFunc
	WaitGroup sync.WaitGroup
	Redis     *redis.Client
	MysqlDao  *dao.DAO
	// gRPC client initialized via gsv/cli
	JobCli    job.ServiceClient
}

// NewUseCaseContext initializes and returns a singleton UseCaseContext.
func NewUseCaseContext(c conf.Config) *UseCaseContext {
	// ... (initialization logic)

	// Inject DAO
	dsc.MysqlDao = dao.NewDataAccess(...)
	
	// Inject Redis Client
	dsc.Redis = redis.NewClient(...)

	// Initialize gRPC client using gsv/cli
	opts := cli.Classic()
	cc, err := cli.NewClient(discovery.Scheme("go-job"), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
	dsc.JobCli = job.NewServiceClient(cc.Conn())
	
	// ... (other initializations)
	return dsc
}
```

### 2.2 Domain Implementation (`domain/<domain_name>/`)

Each business domain resides in a separate directory. The recommended structure includes entities, use cases, and a repository.

**Quick Start**: Use the `gsv` CLI tool to rapidly create a domain skeleton.
```shell
gsv gen domain <domain_name>
```
For example, `gsv gen domain example` will create the following structure:

#### `example.go` (Entity Definition)
```golang
package example

type Example struct {
	Id   int
	// ... other fields
}
```

#### `usecase.go` (Use Case Implementation)
A use case encapsulates a specific business process, calling the repository to persist data.
```golang
package example

import "xxx/internal/domain"

type UseCase struct {
	ctx  *domain.UseCaseContext
	repo *repo
}

func NewUseCase(ctx *domain.UseCaseContext) *UseCase {
	return &UseCase{
		ctx:  ctx,
		repo: newRepo(), // repo is typically stateless
	}
}

func (uc *UseCase) Create(ctx context.Context, example *Example) error {
	return uc.repo.Create(ctx, example)
}
// ... other use case methods: Update, Delete, Get, Query
```

#### `repo.go` (Repository Implementation)
The repository is responsible for interacting with data storage. In simple scenarios, it can directly implement data access logic.
```golang
package example

// The repo struct is typically stateless.
type repo struct {}

func newRepo() *repo {
	return &repo{}
}

// Create persists an Example entity.
func (r *repo) Create(ctx context.Context, example *Example) error {
	// Implement database insertion logic here.
	// For example: return domain.GetUseCaseContext().MysqlDao.Create(example)
	return nil
}
// ... other data access methods
```

## 3. Delivery Layer (`internal/delivery`)

The Delivery layer acts as the bridge
between the outside world and the domain use cases.

### 3.1 Bootstrap File (`delivery/bootstrap.go`)

This file is used to initialize the server instance and register all delivery services (gRPC, HTTP, etc.).

```golang
package delivery

import (
	"github.com/ringbrew/gsv/server"
	"github.com/ringbrew/gsv/service"
	"{{moduleName}}/internal/domain"
)

// NewServer creates a gsv server instance.
func NewServer(ctx *domain.UseCaseContext) server.Server {
	// ... (server configuration)
	return server.NewServer(server.GRPC, &opt)
}

// ServiceList registers all services that implement the service.Service interface.
func ServiceList(ctx *domain.UseCaseContext) []service.Service {
	return []service.Service{
		NewExampleService(ctx), // Register gRPC service
		NewApiService(ctx),     // Register HTTP service
		// ... add more services
	}
}
```

### 3.2 gRPC Service

`gsv` provides convenient tools for generating and implementing gRPC services.

**Workflow**:
1.  Write a `.proto` file in the `proto/` directory. **Note**: The `go_package` option must point to the `export/` directory.
2.  Run the `gsv gen grpc` command. This will automatically generate gRPC code in `export/` and create a service skeleton in `delivery/`.
3.  Implement the business logic in the generated service file by calling the corresponding domain use cases.

**`proto/example.proto` Example**:
```proto
syntax = "proto3";
package example;

// The go_package option must point to the export directory for external reference.
option go_package = "{{moduleName}}/export/example";

service Service {
    rpc Create(Example) returns (OpResp){};
    // ...
}
```

**Generated Service Skeleton `delivery/example/service.go`**:
```golang
package example

import (
	"context"
	"github.com/ringbrew/gsv/service"
	pb " {{moduleName}}/export/example" // Import generated protobufs
	"{{moduleName}}/internal/domain"
	domain_example "{{moduleName}}/internal/domain/example"
)

type Service struct {
	pb.UnimplementedServiceServer
	ctx *domain.UseCaseContext
	// ...
}

func NewService(ctx *domain.UseCaseContext) service.Service {
	// ... (initialization)
}

// Create implements the gRPC interface and calls the domain use case.
func (s *Service) Create(ctx context.Context, req *pb.Example) (*pb.OpResp, error) {
	uc := domain_example.NewUseCase(s.ctx)
	
	// Data transformation: pb.Example -> domain_example.Example
	domainModel := &domain_example.Example{ /* ... */ }

	if err := uc.Create(ctx, domainModel); err != nil {
		return nil, err // Errors will be handled by the gsv framework
	}

	return &pb.OpResp{Code: 200, Msg: "Success"}, nil
}
```

### 3.3 HTTP Service

`gsv` also supports the rapid creation of HTTP services.

**Workflow**:
1.  Run `gsv gen http <service_name>` to create an HTTP service and handler skeleton.
2.  Define routes and handler functions in `handler.go`.
3.  The handler functions call domain use cases to complete business logic.

**`delivery/api/handler.go` Example**:
```golang
package api

import (
	"github.com/ringbrew/gsv/service"
	"{{moduleName}}/internal/domain"
	"net/http"
)

type Handler struct {
	ctx *domain.UseCaseContext
}

func NewHandler(ctx *domain.UseCaseContext) *Handler {
	return &Handler{ctx: ctx}
}

// Query is an example handler function.
func (h *Handler) Query(w http.ResponseWriter, r *http.Request) {
	// 1. Parse request parameters.
	// 2. Call a domain use case.
	// 3. Serialize and return the response.
}

// HttpRoute defines all HTTP routes.
func (h *Handler) HttpRoute() []service.HttpRoute {
	return []service.HttpRoute{
		service.NewHttpRoute(http.MethodGet, "/example", h.Query, service.HttpMeta{
			Remark: "Query example entities",
		}),
		// ... other routes
	}
}
```