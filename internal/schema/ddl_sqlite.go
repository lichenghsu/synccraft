package schema

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/lichenghsu/synccraft/internal/dialect"
)

type sqliteDDL struct{ d dialect.SQLite }

var (
	// sqliteBareDefault matches defaults SQLite accepts without parentheses.
	sqliteBareDefault = regexp.MustCompile(`(?i)^('.*'|[xX]'[0-9a-fA-F]*'|[-+]?[0-9.]+([eE][-+]?[0-9]+)?|null|true|false|current_(time|date|timestamp)|\(.*\))$`)
	// sqliteConstDefault matches defaults ALTER TABLE ADD COLUMN accepts.
	sqliteConstDefault = regexp.MustCompile(`(?i)^('.*'|[xX]'[0-9a-fA-F]*'|[-+]?[0-9.]+([eE][-+]?[0-9]+)?|null|true|false)$`)
)

// SQLite's ALTER TABLE only adds, drops and renames columns. Everything else
// rebuilds the table: create the new definition under a temporary name, copy
// the shared columns, drop the old table, rename, recreate indexes.
func (g sqliteDDL) statements(cur *Table, c Change) ([]string, error) {
	d := g.d
	t := qualified(d, c.Table)

	switch c.Kind {
	case AddTable:
		return createTable(d, t, c.Create, g.columnDef, g.createIndex(c.Table))
	case DropTable:
		return []string{"DROP TABLE " + t}, nil
	case AddColumn:
		col := *c.Column
		if col.Nullable && col.Default == nil || col.Default != nil && sqliteConstDefault.MatchString(*col.Default) {
			return []string{"ALTER TABLE " + t + " ADD COLUMN " + g.columnDef(col)}, nil
		}
		return g.rebuild(cur, c)
	case DropColumn:
		if g.inKeyOrConstraint(cur, c.Column.Name) {
			return g.rebuild(cur, c)
		}
		return []string{"ALTER TABLE " + t + " DROP COLUMN " + d.QuoteIdent(c.Column.Name)}, nil
	case AlterColumn, AlterPrimaryKey:
		return g.rebuild(cur, c)
	case AddIndex:
		if err := checkIndexColumns(*c.Index); err != nil {
			return nil, err
		}
		if c.Index.Constraint {
			return g.rebuild(cur, c)
		}
		return []string{g.createIndex(c.Table)(*c.Index)}, nil
	case DropIndex:
		if c.Index.Constraint {
			return g.rebuild(cur, c)
		}
		return []string{"DROP INDEX " + d.QuoteIdent(c.Index.Name)}, nil
	}
	return nil, fmt.Errorf("unknown change %q", c.Kind)
}

func (g sqliteDDL) createIndex(t dialect.Table) func(dialect.Index) string {
	return func(ix dialect.Index) string { return createIndexSQL(g.d, t, ix) }
}

func (g sqliteDDL) columnDef(c dialect.Column) string {
	s := g.d.QuoteIdent(c.Name)
	if c.Type != "" {
		s += " " + c.Type
	}
	s += nullClause(c)
	if c.Default != nil {
		v := *c.Default
		if !sqliteBareDefault.MatchString(v) {
			v = "(" + v + ")"
		}
		s += " DEFAULT " + v
	}
	return s
}

func (g sqliteDDL) inKeyOrConstraint(t *Table, column string) bool {
	if slices.Contains(t.PrimaryKey, column) {
		return true
	}
	for _, ix := range t.Indexes {
		if ix.Constraint && slices.Contains(ix.Columns, column) {
			return true
		}
	}
	return false
}

func (g sqliteDDL) rebuild(cur *Table, c Change) ([]string, error) {
	d := g.d
	next := cloneTable(cur)
	applyChange(map[dialect.Table]*Table{c.Table: next}, c)

	tmp := d.QuoteIdent(c.Table.Name + "__synccraft_new")
	create, err := createTable(d, tmp, next, g.columnDef, g.createIndex(c.Table))
	if err != nil {
		return nil, err
	}

	var shared []string
	for _, col := range next.Columns {
		if slices.ContainsFunc(cur.Columns, func(x dialect.Column) bool { return x.Name == col.Name }) {
			shared = append(shared, col.Name)
		}
	}
	t := qualified(d, c.Table)
	out := []string{create[0]}
	if len(shared) > 0 {
		cols := quoteList(d, shared)
		out = append(out, "INSERT INTO "+tmp+" ("+cols+") SELECT "+cols+" FROM "+t)
	}
	out = append(out,
		"DROP TABLE "+t,
		"ALTER TABLE "+tmp+" RENAME TO "+d.QuoteIdent(c.Table.Name),
	)
	return append(out, create[1:]...), nil
}
