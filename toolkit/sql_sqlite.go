package toolkit

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"

	"modernc.org/sqlite" // pure-Go SQLite driver
)

// sqlitePragmaReadOnly lists pragmas that only report. All others are denied:
// they change connection or file state (query_only, journal_mode, writable_schema, ...).
var sqlitePragmaReadOnly = map[string]bool{
	"table_info": true, "table_xinfo": true, "table_list": true,
	"index_list": true, "index_info": true, "index_xinfo": true,
	"foreign_key_list": true, "database_list": true, "collation_list": true,
	"function_list": true, "module_list": true, "pragma_list": true,
	"compile_options": true, "page_count": true, "freelist_count": true,
}

// sqliteAuthorizer returns the policy for one SQLite connection. It always
// denies ATTACH, DETACH, load_extension and pragmas that are not read-only
// reports. With readOnly it also denies every action that writes.
func sqliteAuthorizer(readOnly bool) sqlite.AuthorizerFn {
	return func(action sqlite.AuthorizerActionCode, arg1, arg2, _, _ string) sqlite.AuthorizerReturnCode {
		switch action {
		case sqlite.AuthAttach, sqlite.AuthDetach:
			return sqlite.AuthorizerDeny
		case sqlite.AuthFunction:
			if strings.EqualFold(arg2, "load_extension") {
				return sqlite.AuthorizerDeny
			}
		case sqlite.AuthPragma:
			// arg1 = pragma name, arg2 = its argument (table name for table_info). Listed pragmas ignore arguments.
			if !sqlitePragmaReadOnly[strings.ToLower(arg1)] {
				return sqlite.AuthorizerDeny
			}
		case sqlite.AuthCreateIndex, sqlite.AuthCreateTable, sqlite.AuthCreateTempIndex,
			sqlite.AuthCreateTempTable, sqlite.AuthCreateTempTrigger, sqlite.AuthCreateTempView,
			sqlite.AuthCreateTrigger, sqlite.AuthCreateView, sqlite.AuthDelete,
			sqlite.AuthDropIndex, sqlite.AuthDropTable, sqlite.AuthDropTempIndex,
			sqlite.AuthDropTempTable, sqlite.AuthDropTempTrigger, sqlite.AuthDropTempView,
			sqlite.AuthDropTrigger, sqlite.AuthDropView, sqlite.AuthInsert, sqlite.AuthUpdate,
			sqlite.AuthAlterTable, sqlite.AuthReindex, sqlite.AuthAnalyze,
			sqlite.AuthCreateVTable, sqlite.AuthDropVTable:
			if readOnly {
				return sqlite.AuthorizerDeny
			}
		}
		return sqlite.AuthorizerOK
	}
}

// openSQLite opens dsn on a private driver whose every pooled connection gets
// sqliteAuthorizer(readOnly), and PRAGMA query_only=1 when readOnly.
func openSQLite(dsn string, readOnly bool) *sql.DB {
	return openSQLiteOn(&sqlite.Driver{}, dsn, readOnly, nil)
}

// openSQLiteOn is openSQLite on a caller-built private driver (the Db toolkit
// registers functions on it). policy replaces sqliteAuthorizer when not nil.
func openSQLiteOn(drv *sqlite.Driver, dsn string, readOnly bool, policy func(readOnly bool) sqlite.AuthorizerFn) *sql.DB {
	if policy == nil {
		policy = sqliteAuthorizer
	}
	drv.RegisterConnectionHook(func(c sqlite.ExecQuerierContext, _ string) error {
		if readOnly {
			if _, err := c.ExecContext(context.Background(), "PRAGMA query_only = 1", nil); err != nil {
				return err
			}
		}
		reg, ok := c.(sqlite.AuthorizerRegisterer)
		if !ok {
			return fmt.Errorf("sqlite driver does not support an authorizer")
		}
		return reg.RegisterAuthorizer(policy(readOnly))
	})
	return sql.OpenDB(sqliteConnector{drv, dsn})
}

type sqliteConnector struct {
	drv *sqlite.Driver
	dsn string
}

func (c sqliteConnector) Connect(context.Context) (driver.Conn, error) { return c.drv.Open(c.dsn) }
func (c sqliteConnector) Driver() driver.Driver                        { return c.drv }
