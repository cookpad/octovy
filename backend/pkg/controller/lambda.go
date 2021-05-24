package controller

import (
	"github.com/aws/aws-lambda-go/events"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"
	"github.com/gin-gonic/gin"
	"github.com/m-mizutani/goerr"
	"github.com/m-mizutani/golambda"
	"github.com/m-mizutani/octovy/backend/pkg/api"
	"github.com/m-mizutani/octovy/backend/pkg/domain/model"
	"github.com/m-mizutani/octovy/backend/pkg/usecase"
)

func (x *Controller) LambdaAPIHandler(event golambda.Event) (interface{}, error) {
	var req events.APIGatewayProxyRequest
	if err := event.Bind(&req); err != nil {
		return nil, golambda.WrapError(err).With("event", event)
	}

	gin.SetMode(gin.ReleaseMode)
	engine := api.New(&api.Config{
		Usecase:  x.Usecase,
		AssetDir: "assets",
	})

	ginLambda := ginadapter.New(engine)

	return ginLambda.Proxy(req)
}

func (x *Controller) LambdaScanRepo(event golambda.Event) (interface{}, error) {
	records, err := event.DecapSQSBody()
	if err != nil {
		return nil, goerr.Wrap(err).With("event", event)
	}

	x.Config.TrivyDBPath = "/tmp/trivy.db"

	uc := usecase.New(x.Config)
	for _, record := range records {
		var req model.ScanRepositoryRequest
		if err := record.Bind(&req); err != nil {
			return nil, err
		}

		if err := uc.ScanRepository(&req); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (x *Controller) LambdaUpdateDB() (interface{}, error) {
	if err := usecase.New(x.Config).UpdateTrivyDB(); err != nil {
		return nil, err
	}
	return nil, nil
}
