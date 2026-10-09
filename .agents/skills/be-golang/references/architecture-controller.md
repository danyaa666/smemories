# Controller

## Overview

Controller is the entry point of the service. It is responsible for handling incoming requests, validate input, coordinate Manager calls, format response and return.

## File Structure
```
internal/controller/
  <service-name>_service/
    <module-name>_router.go
    middleware/
      <middleware-name>_middleware.go
    controller_helper/
      <purpose>_helper.go
```

- Each service has its own folder in controller folder.
- Each module have it own router file, which will handle all request for that module. But keep the modules size small (< 500 lines), if module is too big, split it into smaller modules.
- For common function that need to be used in multiple modules, create a helper file in controller_helper folder.
- For common action which will execute during request/response processing, create a middleware file in middleware folder.

## Helper

Helper is function that used by multiple modules in controller. If one logic repeat in multiple modules, extract it to helper.

### Example:
```go
func ParseStringToUint32(str string) (uint32, error) {
    if str == "" {
        return 0, errors.NewError(errors.ErrorParam, "string is empty")
    }
    val, err := strconv.ParseUint(str, 10, 32)
    if err != nil {
        return 0, errors.NewError(errors.ErrorParam, "string is not a valid uint32")
    }
    return uint32(val), nil
}
```

### Rules
- Controller Helper function should be small (<30 lines) and focused on one task. It should not contain any business logic.
- Multiple Controller Helper function can be group into single file if they are related to each other (Like `number_parser_helper.go`, `time_format_helper.go`, etc).

## Middleware

Middleware is a function that will execute before or after the request is processed by controller.

### Example:
```go
func WithTraceId(c *gin.Context) {
	// Generate trace id
	traceId, err := uuid.NewUUID()
	if err != nil {
		zap.L().With(zap.Error(err)).Warn("Error while request new requestId")
	}

	trace_id.SetGinTraceId(c, traceId.String())
	c.Next()
}
```

```go

func NewHttpClientLoggerMiddleware(logOpts ...log.RequestLogOption) gin.HandlerFunc {
    // init config
	cfg := &log.RequestLogConfig{}
	cfg.Apply(logOpts...)
    return func(c *gin.Context) {
        // Log request
        c.Next()
        // Log response
    }
}
```

### Middleware Naming Convention
- Middleware function with no params must start with `With` prefix, follow by action it perform. 
- Middleware function with params must start with `New` prefix and return `gin.HandlerFunc`.

### Required Middleware
For HTTP service, we should apply at least below middlewares (can be found in `go-common/http_middleware`):
- WithPanicRecover: recover from panic and return error response
- WithTraceId: add trace id to context
- NewHttpTimeoutMiddleware: add timeout to context
- WithMetricsMiddleware: export request metrics (duration, status code, etc.) to monitor service
- NewHttpClientLoggerMiddleware: log request and response
- WithGzipMiddleware: decode request if it use gzip
- WithIpAddress: extract ip address from request and add to context

For Grpc service, we should apply at least below middlewares (can be found in `go-common/grpc_middleware`):
- WithPanicRecoverInterceptor: recover from panic and return error response
- NewTraceIdMiddleware: add trace id to context
- NewGrpcMetricsInterceptor: export request metrics (duration, status code, etc.) to monitor service
- NewLoggerInterceptor: log request and response


## Controller main file

Controller focuses on parsing requests and returning the format defined in the TD. It may need to coordinate with multiple Managers to complete a requirement.

### Create/Update data action

- Call `req.Validate()` first; return `errors.ErrorParam` on failure.
- Parse proto/HTTP request fields into Manager input types.
- Trigger Manager action to create/update data.
- Map Manager output to proto/HTTP response types.

Example:
```go
func (s *OrderServiceServer) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.CreateOrderResponse, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, errors.WithErrorCode(errors.ErrorParam, err)
	}

	// Parse proto/HTTP request fields into Manager input types
	order := &order.Order{
		UserID: req.UserId,
		Amount: req.Amount,
	}

	// Delegate entirely to Manager
	createdOrder, err := s.orderManager.CreateOrder(ctx, order)
	if err != nil {
		return nil, err
	}

	// Map Manager output to proto/HTTP response types
	return &pb.CreateOrderResponse{
		OrderId: createdOrder.ID,
		Status:  createdOrder.Status,
	}, nil
}
```

### Get data action
- Call `req.Validate()` first; return `errors.ErrorParam` on failure.
- Parse proto/HTTP request fields into Manager input types.
- Trigger Manager action to get data.
- Map Manager output to proto/HTTP response types. If mapping is complex or may reuse in multiple places, create a parser function to map.

Example:
```go
func (s *OrderServiceServer) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.GetOrderResponse, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, err
	}

    orderFilter := &order.Filter{
        ID: req.OrderId,
    }

	// Delegate entirely to Manager
	foundOrder, err := s.orderManager.GetOrder(ctx, orderFilter)
	if err != nil {
		return nil, err
	}

    pbOrder, err := parseOrderToPb(ctx, foundOrder)
	if err != nil {
		return nil, err
	}
	return pbOrder, nil
}
```
