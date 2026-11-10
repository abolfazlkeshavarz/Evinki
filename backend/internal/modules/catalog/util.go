package catalog

import (
	"strconv"

	"github.com/jackc/pgx/v5"
)

type pgxTx = pgx.Tx

func intStr(n int) string { return strconv.Itoa(n) }
