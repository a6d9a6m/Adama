package service

import (
	stdhttp "net/http"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/littleSand/adama/app/ops-ai/internal/biz"
)

type OpsAIService struct {
	uc  *biz.OpsAIUsecase
	log *log.Helper
}

func NewOpsAIService(uc *biz.OpsAIUsecase, logger log.Logger) *OpsAIService {
	return &OpsAIService{
		uc:  uc,
		log: log.NewHelper(logger),
	}
}

func (s *OpsAIService) Ask(ctx khttp.Context) error {
	var req biz.AskRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	response, err := s.uc.Ask(ctx, req)
	if err != nil {
		return err
	}
	return ctx.JSON(stdhttp.StatusOK, response)
}
