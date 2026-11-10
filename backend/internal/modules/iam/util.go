package iam

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

type pgxTx = pgx.Tx

func setRolesTx(r *http.Request, tx pgx.Tx, uid int64, roles []string) error {
	if _, err := tx.Exec(r.Context(), `DELETE FROM user_roles WHERE user_id=$1`, uid); err != nil {
		return err
	}
	for _, role := range roles {
		if _, err := tx.Exec(r.Context(), `INSERT INTO user_roles(user_id,role_code) VALUES($1,$2)`, uid, role); err != nil {
			return err
		}
	}
	return nil
}
