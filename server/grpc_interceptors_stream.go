package server

import (
	"google.golang.org/grpc"
)

func recoverStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		//defer func() {
		//	if p := recover(); p != nil {
		//		logger.Fatal(logger.NewEntry(ctx).WithMessage(fmt.Sprintf("server panic: %v", p)))
		//		err = status.Errorf(codes.Internal, "%s", p)
		//	}
		//}()
		//
		//return handler(srv, ss)
		return nil
	}
}

func logStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		//startTime := time.Now()

		//resp, err := handler(ctx, req)
		//duration := time.Since(startTime)
		//
		//code := grpcLogging.DefaultErrorToCode(err)
		//entry := logger.NewEntry(ctx)
		//entry = entry.WithExtra("method", info.FullMethod).
		//	WithExtra("latency", duration.String()).
		//	WithExtra("codeString", code.String()).
		//	WithExtra("code", strconv.Itoa(int(code)))
		//
		//if err != nil {
		//	logger.Error(entry.WithMessage(err.Error()))
		//} else {
		//	logger.Info(entry.WithMessage("success"))
		//}

		//return resp, err
		return nil
	}
}
