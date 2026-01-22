package biz

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
)

// 获取用户信息的时候使用
type User struct {
	Id       int64
	Username string
}

// 注册用户信息的时候使用
type RegUser struct {
	Username     string
	PasswordHash string
}

// 更新用户信息的时候使用
type UpdateUser struct {
	PasswordHash string
}

type UserAddress struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Consignee string `json:"consignee"`
	Phone     string `json:"phone"`
	Province  string `json:"province"`
	City      string `json:"city"`
	Detail    string `json:"detail"`
	IsDefault bool   `json:"is_default"`
}

type UserRepo interface {
	GetUser(ctx context.Context, id int64) (*User, error)
	CreateUser(ctx context.Context, user *RegUser) (id int64, err error)
	UpdateUser(ctx context.Context, id int64, user *UpdateUser) error
	DeleteUser(ctx context.Context, id int64) error
	ListAddresses(ctx context.Context, userID int64) ([]UserAddress, error)
	CreateAddress(ctx context.Context, address *UserAddress) (int64, error)
	UpdateAddress(ctx context.Context, address *UserAddress) error
	DeleteAddress(ctx context.Context, userID int64, addressID int64) error
	SetDefaultAddress(ctx context.Context, userID int64, addressID int64) error
}

type UserUsecase struct {
	repo UserRepo
	log  *log.Helper
}

func NewUserUsecase(repo UserRepo, logger log.Logger) *UserUsecase {
	return &UserUsecase{repo: repo, log: log.NewHelper(logger)}
}

func (uc *UserUsecase) Get(ctx context.Context, id int64) (*User, error) {
	return uc.repo.GetUser(ctx, id)
}

func (uc *UserUsecase) Create(ctx context.Context, user *RegUser) (id int64, err error) {
	return uc.repo.CreateUser(ctx, user)
}

func (uc *UserUsecase) Update(ctx context.Context, id int64, user *UpdateUser) error {
	return uc.repo.UpdateUser(ctx, id, user)
}

func (uc *UserUsecase) Delete(ctx context.Context, id int64) error {
	return uc.repo.DeleteUser(ctx, id)
}

func (uc *UserUsecase) ListAddresses(ctx context.Context, userID int64) ([]UserAddress, error) {
	return uc.repo.ListAddresses(ctx, userID)
}

func (uc *UserUsecase) CreateAddress(ctx context.Context, address *UserAddress) (int64, error) {
	return uc.repo.CreateAddress(ctx, address)
}

func (uc *UserUsecase) UpdateAddress(ctx context.Context, address *UserAddress) error {
	return uc.repo.UpdateAddress(ctx, address)
}

func (uc *UserUsecase) DeleteAddress(ctx context.Context, userID int64, addressID int64) error {
	return uc.repo.DeleteAddress(ctx, userID, addressID)
}

func (uc *UserUsecase) SetDefaultAddress(ctx context.Context, userID int64, addressID int64) error {
	return uc.repo.SetDefaultAddress(ctx, userID, addressID)
}
