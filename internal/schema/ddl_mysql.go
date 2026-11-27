package schema

import (
	"fmt"
	"regexp"

	"github.com/lichenghsu/synccraft/internal/dialect"
)

type mysqlDDL struct{ d dialect.MySQL }

// mysqlBareDefault matches defaults MySQL accepts without parentheses:
// string literals, numbers, NULL, TRUE/FALSE and the current-time functions.
var mysqlBareDefault = regexp.MustCompile(`(?i)^('.*'|-?[0-9.]+|null|true|false|(current_timestamp|now|localtime|localtimestamp)(\(\d*\))?|\(.*\))$`)

func (g mysqlDDL) statements(cur *Table, c Change) ([]string, error) {
	d := g.d
	t := qualified(d, c.Table)

	switch c.Kind {
	case AddTable:
		return createTable(d, t, c.Create, g.columnDef, func(ix dialect.Index) string { return createIndexSQL(d, c.Table, ix) })
	case DropTable:
		return []string{"DROP TABLE " + t}, nil
	case AddColumn:
		pos := " FIRST"
		if c.After != "" {
			pos = " AFTER " + d.QuoteIdent(c.After)
		}
		return []string{"ALTER TABLE " + t + " ADD COLUMN " + g.columnDef(*c.Column) + pos}, nil
	case DropColumn:
		return []string{"ALTER TABLE " + t + " DROP COLUMN " + d.QuoteIdent(c.Column.Name)}, nil
	case AlterColumn:
		return []string{"ALTER TABLE " + t + " MODIFY COLUMN " + g.columnDef(*c.Column)}, nil
	case AlterPrimaryKey:
		// One statement, so an AUTO_INCREMENT column is never left without a key.
		switch {
		case len(c.FromPrimaryKey) > 0 && len(c.PrimaryKey) > 0:
			return []string{"ALTER TABLE " + t + " DROP PRIMARY KEY, ADD PRIMARY KEY (" + quoteList(d, c.PrimaryKey) + ")"}, nil
		case len(c.PrimaryKey) > 0:
			return []string{"ALTER TABLE " + t + " ADD PRIMARY KEY (" + quoteList(d, c.PrimaryKey) + ")"}, nil
		default:
			return []string{"ALTER TABLE " + t + " DROP PRIMARY KEY"}, nil
		}
	case AddIndex:
		if err := checkIndexColumns(*c.Index); err != nil {
			return nil, err
		}
		return []string{createIndexSQL(d, c.Table, *c.Index)}, nil
	case DropIndex:
		return []string{"DROP INDEX " + d.QuoteIdent(c.Index.Name) + " ON " + t}, nil
	}
	return nil, fmt.Errorf("unknown change %q", c.Kind)
}

// columnDef wraps expression defaults in parentheses; MySQL reports them
// bare (uuid()) but only accepts them parenthesized.
func (g mysqlDDL) columnDef(c dialect.Column) string {
	s := g.d.QuoteIdent(c.Name) + " " + c.Type + nullClause(c)
	if c.AutoIncrement {
		s += " AUTO_INCREMENT"
	}
	if c.Default != nil {
		v := *c.Default
		if !mysqlBareDefault.MatchString(v) {
			v = "(" + v + ")"
		}
		s += " DEFAULT " + v
	}
	return s
}
