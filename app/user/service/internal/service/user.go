package service

import (
	"context"
	stdhttp "net/http"
	"strconv"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
	pb "github.com/littleSand/adama/api/user/service/v1"
	"github.com/littleSand/adama/app/user/service/internal/biz"
	"github.com/littleSand/adama/pkg/requestctx"
)

// 创建用户
func (s *UserService) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserReply, error) {
	// u := &biz.RegUser{Username: req.Username, PasswordHash: req.Password}
	// rv, err := s.uc.Create(ctx, u)

	// s.log.Infof("service/user/CreateUser: rv: %v, err: %v", rv, err)

	return &pb.CreateUserReply{}, nil
}
func (s *UserService) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserReply, error) {
	u := &biz.UpdateUser{
		PasswordHash: req.Password,
	}

	err := s.uc.Update(ctx, req.Id, u)

	if err != nil {
		// 返回错误信息
		return nil, err
	}
	return &pb.UpdateUserReply{}, nil
}

func (s *UserService) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserReply, error) {
	return &pb.DeleteUserReply{}, nil
}

func (s *UserService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserReply, error) {
	rv, err := s.uc.Get(ctx, req.Id)
	s.log.Info("getuser request: ", rv, req)
	return &pb.GetUserReply{
		Id:       rv.Id,
		Username: rv.Username,
	}, err
}

func (s *UserService) ListUser(ctx context.Context, req *pb.ListUserRequest) (*pb.ListUserReply, error) {
	return &pb.ListUserReply{}, nil
}

type addressPayload struct {
	Consignee string `json:"consignee"`
	Phone     string `json:"phone"`
	Province  string `json:"province"`
	City      string `json:"city"`
	Detail    string `json:"detail"`
	IsDefault bool   `json:"is_default"`
}

func (s *UserService) ListAddressesHTTP(ctx khttp.Context) error {
	userID := resolveUserID(ctx)
	addresses, err := s.uc.ListAddresses(ctx, userID)
	if err != nil {
		return err
	}
	return ctx.JSON(stdhttp.StatusOK, map[string]interface{}{"items": addresses})
}

func (s *UserService) CreateAddressHTTP(ctx khttp.Context) error {
	userID := resolveUserID(ctx)
	var payload addressPayload
	if err := ctx.Bind(&payload); err != nil {
		return err
	}

	id, err := s.uc.CreateAddress(ctx, &biz.UserAddress{
		UserID:    userID,
		Consignee: payload.Consignee,
		Phone:     payload.Phone,
		Province:  payload.Province,
		City:      payload.City,
		Detail:    payload.Detail,
		IsDefault: payload.IsDefault,
	})
	if err != nil {
		return err
	}

	return ctx.JSON(stdhttp.StatusCreated, map[string]interface{}{"id": id})
}

func (s *UserService) UpdateAddressHTTP(ctx khttp.Context) error {
	userID := resolveUserID(ctx)
	addressID, err := strconv.ParseInt(ctx.Vars().Get("id"), 10, 64)
	if err != nil {
		return err
	}

	var payload addressPayload
	if err := ctx.Bind(&payload); err != nil {
		return err
	}

	err = s.uc.UpdateAddress(ctx, &biz.UserAddress{
		ID:        addressID,
		UserID:    userID,
		Consignee: payload.Consignee,
		Phone:     payload.Phone,
		Province:  payload.Province,
		City:      payload.City,
		Detail:    payload.Detail,
		IsDefault: payload.IsDefault,
	})
	if err != nil {
		return err
	}
	return ctx.JSON(stdhttp.StatusOK, map[string]interface{}{"updated": true})
}

func (s *UserService) DeleteAddressHTTP(ctx khttp.Context) error {
	userID := resolveUserID(ctx)
	addressID, err := strconv.ParseInt(ctx.Vars().Get("id"), 10, 64)
	if err != nil {
		return err
	}

	if err := s.uc.DeleteAddress(ctx, userID, addressID); err != nil {
		return err
	}
	return ctx.JSON(stdhttp.StatusOK, map[string]interface{}{"deleted": true})
}

func (s *UserService) SetDefaultAddressHTTP(ctx khttp.Context) error {
	userID := resolveUserID(ctx)
	addressID, err := strconv.ParseInt(ctx.Vars().Get("id"), 10, 64)
	if err != nil {
		return err
	}

	if err := s.uc.SetDefaultAddress(ctx, userID, addressID); err != nil {
		return err
	}
	return ctx.JSON(stdhttp.StatusOK, map[string]interface{}{"updated": true})
}

func resolveUserID(ctx khttp.Context) int64 {
	if userID, ok := requestctx.UserID(ctx); ok && userID > 0 {
		return userID
	}
	if raw := ctx.Request().URL.Query().Get("user_id"); raw != "" {
		if userID, err := strconv.ParseInt(raw, 10, 64); err == nil && userID > 0 {
			return userID
		}
	}
	return 0
}
