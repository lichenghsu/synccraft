package dialect

import (
	"fmt"

	"github.com/lichenghsu/synccraft/internal/conn"
)

// For returns the dialect for a connection type. MariaDB uses MySQL.
func For(t conn.DBType) (Dialect, error) {
	switch t {
	case conn.SQLite:
		return SQLite{}, nil
	case conn.PostgreSQL:
		return PostgreSQL{}, nil
	case conn.MySQL:
		return MySQL{}, nil
	case conn.SQLServer:
		return SQLServer{}, nil
	}
	return nil, fmt.Errorf("%w: %q", conn.ErrUnsupportedType, t)
}
