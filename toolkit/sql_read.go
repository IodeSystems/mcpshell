package toolkit

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"net/url"

	"modernc.org/sqlite"
)

// readQuery runs a query that must not write, whatever its text says. The
// returned done closes the rows and releases what the query held (rollback of
// the read-only transaction, or the connection back to read-write mode).
func (st *sqlToolkit) readQuery(ctx context.Context, query string, params []any) (*sql.Rows, func(), error) {
	switch {
	case st.dialect == dialectPostgres:
		// The server rejects writes in a READ ONLY transaction.
		tx, err := st.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, nil, err
		}
		rows, err := tx.QueryContext(ctx, query, params...)
		if err != nil {
			tx.Rollback()
			return nil, nil, err
		}
		return rows, func() { rows.Close(); tx.Rollback() }, nil
	case st.readOnly:
		// Every connection is already read-only.
		rows, err := st.db.QueryContext(ctx, query, params...)
		return rows, func() { rows.Close() }, err
	default:
		// Writable SQLite pool: borrow one connection and make it read-only for
		// this query. A pooled second database would not work for `:memory:`, where
		// each connection is its own database.
		conn, err := st.db.Conn(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := conn.Raw(func(dc any) error { return setSQLiteReadOnly(ctx, dc, true) }); err != nil {
			conn.Close()
			return nil, nil, err
		}
		restore := func() {
			// A connection that cannot be restored is discarded from the pool.
			conn.Raw(func(dc any) error {
				if err := setSQLiteReadOnly(ctx, dc, false); err != nil {
					return driver.ErrBadConn
				}
				return nil
			})
			conn.Close()
		}
		rows, err := conn.QueryContext(ctx, query, params...)
		if err != nil {
			restore()
			return nil, nil, err
		}
		return rows, func() { rows.Close(); restore() }, nil
	}
}

// setSQLiteReadOnly switches one SQLite connection between the read-only and
// read-write policy. The authorizer is removed while query_only changes,
// because the policy denies that pragma.
func setSQLiteReadOnly(ctx context.Context, dc any, ro bool) error {
	reg, ok := dc.(sqlite.AuthorizerRegisterer)
	ex, ok2 := dc.(sqlite.ExecQuerierContext)
	if !ok || !ok2 {
		return driver.ErrBadConn
	}
	if err := reg.RegisterAuthorizer(nil); err != nil {
		return err
	}
	v := "0"
	if ro {
		v = "1"
	}
	if _, err := ex.ExecContext(ctx, "PRAGMA query_only = "+v, nil); err != nil {
		return err
	}
	return reg.RegisterAuthorizer(sqliteAuthorizer(ro))
}

// postgresReadOnlyDSN adds default_transaction_read_only=on to a postgres URL.
// lib/pq sends unknown URL parameters as server runtime parameters.
func postgresReadOnlyDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	q := u.Query()
	q.Set("default_transaction_read_only", "on")
	u.RawQuery = q.Encode()
	return u.String()
}
