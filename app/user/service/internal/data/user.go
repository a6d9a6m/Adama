package data

import (
	"context"
	"database/sql"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/littleSand/adama/app/user/service/internal/biz"
	"github.com/littleSand/adama/pkg/utils/encryption"
)

var _ biz.UserRepo = (*userRepo)(nil)

type userRepo struct {
	data *Data
	log  *log.Helper
}

func (r *userRepo) UpdateUser(ctx context.Context, id int64, user *biz.UpdateUser) error {
	ro, err := r.data.db.User.UpdateOneID(id).
		SetPasswordHash(encryption.Md5Password(user.PasswordHash)).
		Save(ctx)
	r.log.Infof("update-user-result: %v", ro)
	return err
}

func (r *userRepo) DeleteUser(ctx context.Context, id int64) error {
	return r.data.db.User.DeleteOneID(id).Exec(ctx)
}

// TODO: enforce uniqueness for username, phone, and email.
func (r *userRepo) CreateUser(ctx context.Context, user *biz.RegUser) (id int64, err error) {
	po, err := r.data.db.User.Create().
		SetUsername(user.Username).
		SetPasswordHash(encryption.Md5Password(user.PasswordHash)).
		Save(ctx)
	if err != nil {
		return 0, err
	}

	r.log.Infof("data/user/CreateUser: po: %v, err: %v", po, err)
	return po.ID, nil
}

func (r *userRepo) GetUser(ctx context.Context, id int64) (*biz.User, error) {
	po, err := r.data.db.User.Get(ctx, id)
	r.log.Info("data/user: ", po, err)
	if err != nil {
		return nil, err
	}
	return &biz.User{Id: po.ID, Username: po.Username}, nil
}

func NewUserRepo(data *Data, logger log.Logger) biz.UserRepo {
	return &userRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "data/server-service")),
	}
}

func (r *userRepo) ListAddresses(ctx context.Context, userID int64) ([]biz.UserAddress, error) {
	rows, err := r.data.sql.QueryContext(ctx, `
		SELECT id, user_id, consignee, phone, province, city, detail, is_default
		FROM user_addresses
		WHERE user_id = ?
		ORDER BY is_default DESC, id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	addresses := make([]biz.UserAddress, 0)
	for rows.Next() {
		var (
			address   biz.UserAddress
			isDefault bool
		)
		if err := rows.Scan(&address.ID, &address.UserID, &address.Consignee, &address.Phone, &address.Province, &address.City, &address.Detail, &isDefault); err != nil {
			return nil, err
		}
		address.IsDefault = isDefault
		addresses = append(addresses, address)
	}
	return addresses, rows.Err()
}

func (r *userRepo) CreateAddress(ctx context.Context, address *biz.UserAddress) (id int64, err error) {
	tx, err := r.data.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now()
	if address.IsDefault {
		if _, err = tx.ExecContext(ctx, `UPDATE user_addresses SET is_default = 0, updated_at = ? WHERE user_id = ?`, now, address.UserID); err != nil {
			return 0, err
		}
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO user_addresses (user_id, consignee, phone, province, city, detail, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, address.UserID, address.Consignee, address.Phone, address.Province, address.City, address.Detail, address.IsDefault, now, now)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func (r *userRepo) UpdateAddress(ctx context.Context, address *biz.UserAddress) (err error) {
	tx, err := r.data.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now()
	if address.IsDefault {
		if _, err = tx.ExecContext(ctx, `UPDATE user_addresses SET is_default = 0, updated_at = ? WHERE user_id = ?`, now, address.UserID); err != nil {
			return err
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE user_addresses
		SET consignee = ?, phone = ?, province = ?, city = ?, detail = ?, is_default = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, address.Consignee, address.Phone, address.Province, address.City, address.Detail, address.IsDefault, now, address.ID, address.UserID)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	return ensureRowsAffected(result)
}

func (r *userRepo) DeleteAddress(ctx context.Context, userID int64, addressID int64) error {
	result, err := r.data.sql.ExecContext(ctx, `DELETE FROM user_addresses WHERE id = ? AND user_id = ?`, addressID, userID)
	if err != nil {
		return err
	}
	return ensureRowsAffected(result)
}

func (r *userRepo) SetDefaultAddress(ctx context.Context, userID int64, addressID int64) (err error) {
	tx, err := r.data.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now()
	if _, err = tx.ExecContext(ctx, `UPDATE user_addresses SET is_default = 0, updated_at = ? WHERE user_id = ?`, now, userID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE user_addresses SET is_default = 1, updated_at = ? WHERE id = ? AND user_id = ?`, now, addressID, userID)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return ensureRowsAffected(result)
}

func ensureRowsAffected(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
