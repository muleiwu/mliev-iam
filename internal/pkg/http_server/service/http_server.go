package service

import (
	"cnb.cool/mliev/iam/mliev-iam/internal/interfaces"
	"cnb.cool/mliev/iam/mliev-iam/internal/pkg/http_server/impl"
)

type HttpServer struct {
	Helper interfaces.HelperInterface
}

func (receiver *HttpServer) Run() error {

	newHttpServer := impl.NewHttpServer(receiver.Helper)

	newHttpServer.RunHttp()
	return nil
}
