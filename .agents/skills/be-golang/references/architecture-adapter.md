# Adapter

## Overview

Adapter is the connection point of the service to external systems such as database, external API, etc. It is responsible to contact with external systems and return data to Manager.

## File Structure
```
internal/adapter/
  http_client/
    <client-name>/
      <client-module-name>_client.go
      consts.go
      dto.go
  grpc_client/
    <client-name>/
      <client-module-name>_client.go
      consts.go
  database/
    <db-name>/
      <module-name>_repo.go
      repo_manager.go
      consts.go
      model.go
```

- Each type of external system have it own folder in adapter folder (http_client, grpc_client, database).
- Each source in each kind of external system have it own folder in adapter folder.
- If external system have multiple module, create a folder for each module.
- For constants value, put in consts.go file.

## HTTP Client Adapter

HTTP Client adapter is responsible to contact with external API via HTTP protocol and return data to Manager.

- HTTP Client must only export the interface to Manager, not the implementation.
- HTTP Client must not contain any business logic.
- HTTP Client must define it own data struct for request and response in dto.go file.
- HTTP Client must contain at least below middleware (can be found in `go-common/http_client`):
    - WithTimeout: set timeout for request
    - WithRequestParser: set request parser
    - WithResponseParser: set response parser
    - WithLogMiddlewareOpts: log request and response
- HTTP Client must contain a error handler middleware to handle error response from external API.

### Example
``` go
type OrderClient interface {
	GetOrder(ctx context.Context, orderId string) (*GetOrderResponse, error)
}

type orderClientImpl struct {
	client *http_client.HttpClient
	cfg *Config
}

func NewClient(cfg *Config) (OrderClient, error) {
    client, err := http_client.NewHttpClient(
		cfg.BaseUrl,
		http_client.WithTimeout(cfg.Timeout),
		http_client.WithRequestParser(parser.NewJsonParser()),
        http_client.WithResponseParser(parser.NewJsonParser()),
		http_client.WithLogMiddlewareOpts(),
		http_client.WithMiddleware(orderErrorHandler),
	)
	if err != nil {
		return nil, err
	}
	return &orderClientImpl{client: client, cfg: cfg}
}

func orderErrorHandler(r *http.Request, handler http_client_middleware.HttpHandler) (*http.Response, error) {
	resp, err := handler(r)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		jsonParser := parser.NewJsonParser()
		message := ""

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.WrapError(errors.ErrorHttp, err, "read http response failed")
		}
		err = jsonParser.Unmarshal(bodyBytes, &message)
		if err != nil {
			return nil, errors.WrapError(ErrLivechatClient, err, "failed to parse response body")
		}

		return nil, errors.NewError(ErrLivechatClient, fmt.Sprintf("[OrderClient] request failed, status = %v, message = %v", resp.StatusCode, message))
	}
	return resp, nil
}

func (f *orderClientImpl) GetOrder(ctx context.Context, orderId string) (*GetOrderResponse, error) {
    req := &GetOrderRequest{
        OrderId: orderId,
    }
    resp := &GetOrderResponse{}
    header := map[string]string{
        "Content-Type": "application/json",
    }

    err := c.client.Do(
		ctx, http.MethodPost, GetOrderEndpoint, req, resp,
		http_client.WithRequestHeader(header),
	)
	if err != nil {
		return nil, err
	}
    return resp, nil
}
```


## Grpc Client Adapter

Grpc Client adapter is responsible to contact with external API via Grpc protocol and return data to Manager.

- GRPC Client must only export the interface to Manager, not the implementation.
- GRPC Client must use data struct from proto file, not define new data struct.
- GRPC Client must not contain any business logic.
- GRPC Client must contain at least below middleware (can be found in `go-common/grpc_client`):
    - WithTimeout: set timeout for request
    - WithLogMiddlewareOpts: log request and response
- GRPC Client must contain a error handler middleware to handle error response from external API.

### Example
``` go
type OrderClient interface {
	GetOrder(ctx context.Context, orderId string) (*order_pb.GetOrderResponse, error)
}

type orderClientImpl struct {
	client *order_pb.OrderClient
	cfg *Config
}

func NewClient(cfg *Config) (OrderClient, error) {
    grpcClient, err := grpc_client.NewGrpcClient(
		cfg.Host,
		grpc_client.WithLogMiddlewareOpts(),
		grpc_client.WithTimeout(time.Duration(cfg.TimeoutSecond)*time.Second),
		grpc_client.WithInterceptor(NewErrorHandlingInterceptor()),
	)
	if err != nil {
		return nil, err
	}
	return &orderClientImpl{client: order_pb.NewOrderClient(grpcClient.GetConn()), cfg: cfg}
}

func orderErrorHandler() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			return errors.WithErrorCode(ErrOrderClient, err)
		}
		if withMeta, ok := reply.(withResponseMeta); ok {
			meta := withMeta.GetMeta()
			if meta.GetCode() != "" {
				err = errors.NewError(errors.ErrorCode(meta.GetCode()), meta.GetErrorMessage())
			}
		}
		return err
	}
}

func (f *orderClientImpl) GetOrder(ctx context.Context, orderId string) (*order_pb.GetOrderResponse, error) {
    orderResp, err := f.client.GetOrder(ctx, &order_pb.GetOrderRequest{
        OrderId: orderId,
    })
	if err != nil {
		return nil, errors.WrapError(errors.GetErrorCode(err), err, "fail to get agent")
	}
	return orderResp, nil
}
```

## Database Adapter

Database adapter is responsible to contact with database (Mysql, Postgres, ElasticSearch, Clickhouse, etc) and return data to Manager.

- Database adapter must only export the interface to Manager, not the implementation.
- Database adapter must define it own data struct for database table in model.go file.
- Database adapter must not contain any business logic.
- Each error in database adapter must be handled by `database.HandleError` function.
- By default, use slave connection for read operation, master connection for write operation. But if data is really need to be real-time, use master connection for read operation.

### Example
``` go
type OrderRepository interface {
	GetOrder(ctx context.Context, orderId string) (*Order, error)
}

type orderRepositoryImpl struct {
	database.DBConnection
}

func newOrderRepo(db database.DBConnection) OrderRepository {
   return &orderRepositoryImpl{DBConnection: db}
}

func (f *orderRepositoryImpl) GetOrder(ctx context.Context, orderId string) (*Order, error) {
    order := Order{}
	err := f.GetConn(ctx, database.WithSlave()).Where("order_id = ?", orderId).First(&order).Error
	if err != nil {
		return nil, database.HandleError(err)
	}
	return &order, nil
}
```

- All repo must be initialized in `repo_manager.go` file, and return `RepoManager` instance which contain all repo instances.

### Example
``` go

type RepoManager struct {
	database.DBConnection
	OrderRepo OrderRepository
	UserRepo UserRepository
}

func NewRepoManager(db database.DBConnection) RepoManager {
	return &RepoManager{
        DBConnection: db,
		OrderRepo: newOrderRepo(db),
		UserRepo: newUserRepo(db),
	}
}
```
