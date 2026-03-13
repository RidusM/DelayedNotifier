package repository

import pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"

func execOrDB(qe pgxdriver.QueryExecuter, db *pgxdriver.Postgres) pgxdriver.QueryExecuter {
	if qe != nil {
		return qe
	}
	return db
}
