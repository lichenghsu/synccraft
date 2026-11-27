package schema

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lichenghsu/synccraft/internal/dialect"
)

// Step is one Change with the statements that apply it on the target.
type Step struct {
	Change Change   `json:"change"`
	SQL    []string `json:"sql"`
	// Unsupported says why the change cannot be applied automatically. SQL
	// is empty when it is set.
	Unsupported string `json:"unsupported,omitempty"`
}

// generator turns one change into statements for one engine. cur is the
// target table as it stands before the change, nil for AddTable.
type generator interface {
	statements(cur *Table, c Change) ([]string, error)
}

func generatorFor(d dialect.Dialect) (generator, error) {
	switch d := d.(type) {
	case dialect.SQLite:
		return sqliteDDL{d}, nil
	case dialect.PostgreSQL:
		return postgresDDL{d}, nil
	case dialect.MySQL:
		return mysqlDDL{d}, nil
	case dialect.SQLServer:
		return sqlserverDDL{d}, nil
	}
	return nil, fmt.Errorf("no DDL generator for %T", d)
}

// TransactionalDDL reports whether DDL on d can be rolled back. MySQL and
// MariaDB commit implicitly before and after each DDL statement.
func TransactionalDDL(d dialect.Dialect) bool {
	_, isMySQL := d.(dialect.MySQL)
	return !isMySQL
}

// Script returns the statements that apply changes, in order, to a target
// whose current structure is dst. changes may be any subset of Diff's
// result, in Diff's order; each step sees the target as left by the steps
// before it.
func Script(d dialect.Dialect, dst []Table, changes []Change) ([]Step, error) {
	g, err := generatorFor(d)
	if err != nil {
		return nil, err
	}
	state := make(map[dialect.Table]*Table, len(dst))
	for i := range dst {
		state[dst[i].Table] = cloneTable(&dst[i])
	}

	steps := make([]Step, 0, len(changes))
	for _, c := range changes {
		cur := state[c.Table]
		var stmts []string
		switch {
		case c.Kind == AddTable && cur != nil:
			err = errors.New("table already exists in the target")
		case c.Kind != AddTable && cur == nil:
			err = errors.New("table not found in the target")
		default:
			stmts, err = g.statements(cur, c)
		}
		if err != nil {
			steps = append(steps, Step{Change: c, SQL: []string{}, Unsupported: err.Error()})
			continue
		}
		steps = append(steps, Step{Change: c, SQL: stmts})
		applyChange(state, c)
	}
	return steps, nil
}

// Apply runs every statement of steps in one transaction. On engines without
// transactional DDL (see TransactionalDDL) statements before a failure stay
// applied.
func Apply(ctx context.Context, db *sql.DB, steps []Step) error {
	for i, s := range steps {
		if s.Unsupported != "" {
			return fmt.Errorf("step %d (%s %s): %s", i+1, s.Change.Kind, s.Change.Table.Name, s.Unsupported)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, s := range steps {
		for _, stmt := range s.SQL {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("step %d (%s %s): %w\n%s", i+1, s.Change.Kind, s.Change.Table.Name, err, stmt)
			}
		}
	}
	return tx.Commit()
}

// applyChange updates the simulated target after c succeeded.
func applyChange(state map[dialect.Table]*Table, c Change) {
	t := state[c.Table]
	switch c.Kind {
	case AddTable:
		state[c.Table] = cloneTable(c.Create)
	case DropTable:
		delete(state, c.Table)
	case AddColumn:
		at := 0
		if c.After != "" {
			at = slices.IndexFunc(t.Columns, func(x dialect.Column) bool { return x.Name == c.After }) + 1
		}
		t.Columns = slices.Insert(t.Columns, at, *c.Column)
		renumber(t.Columns)
	case DropColumn:
		t.Columns = slices.DeleteFunc(t.Columns, func(x dialect.Column) bool { return x.Name == c.Column.Name })
		renumber(t.Columns)
	case AlterColumn:
		if i := slices.IndexFunc(t.Columns, func(x dialect.Column) bool { return x.Name == c.Column.Name }); i >= 0 {
			pos := t.Columns[i].Position
			t.Columns[i] = *c.Column
			t.Columns[i].Position = pos
		}
	case AlterPrimaryKey:
		t.PrimaryKey = slices.Clone(c.PrimaryKey)
	case AddIndex:
		t.Indexes = append(t.Indexes, *c.Index)
	case DropIndex:
		t.Indexes = slices.DeleteFunc(t.Indexes, func(x dialect.Index) bool { return x.Name == c.Index.Name })
	}
}

func cloneTable(t *Table) *Table {
	c := *t
	c.Columns = sortedColumns(t.Columns)
	c.PrimaryKey = slices.Clone(t.PrimaryKey)
	c.Indexes = slices.Clone(t.Indexes)
	return &c
}

func renumber(cols []dialect.Column) {
	for i := range cols {
		cols[i].Position = i + 1
	}
}

// Shared SQL helpers.

func qualified(d dialect.Dialect, t dialect.Table) string {
	return qualifiedName(d, t.Schema, t.Name)
}

func qualifiedName(d dialect.Dialect, schema, name string) string {
	if schema == "" {
		return d.QuoteIdent(name)
	}
	return d.QuoteIdent(schema) + "." + d.QuoteIdent(name)
}

func quoteList(d dialect.Dialect, names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = d.QuoteIdent(n)
	}
	return strings.Join(q, ", ")
}

func stringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func checkIndexColumns(ix dialect.Index) error {
	if slices.Contains(ix.Columns, "") {
		return fmt.Errorf("index %s has expression key parts, which cannot be recreated", ix.Name)
	}
	return nil
}

// createTable builds CREATE TABLE named name (already quoted) with the
// primary key and UNIQUE constraints inline, followed by CREATE INDEX for
// the other indexes.
func createTable(d dialect.Dialect, name string, t *Table, colDef func(dialect.Column) string, createIndex func(dialect.Index) string) ([]string, error) {
	cols := sortedColumns(t.Columns)
	parts := make([]string, 0, len(cols)+1+len(t.Indexes))
	for _, c := range cols {
		parts = append(parts, colDef(c))
	}
	if len(t.PrimaryKey) > 0 {
		parts = append(parts, "PRIMARY KEY ("+quoteList(d, t.PrimaryKey)+")")
	}
	var after []string
	for _, ix := range sortedIndexes(t.Indexes) {
		if err := checkIndexColumns(ix); err != nil {
			return nil, err
		}
		if ix.Constraint {
			parts = append(parts, "CONSTRAINT "+d.QuoteIdent(ix.Name)+" UNIQUE ("+quoteList(d, ix.Columns)+")")
		} else {
			after = append(after, createIndex(ix))
		}
	}
	stmt := "CREATE TABLE " + name + " (\n  " + strings.Join(parts, ",\n  ") + "\n)"
	return append([]string{stmt}, after...), nil
}

func sortedIndexes(ix []dialect.Index) []dialect.Index {
	ix = slices.Clone(ix)
	slices.SortStableFunc(ix, func(a, b dialect.Index) int { return cmp.Compare(a.Name, b.Name) })
	return ix
}

// createIndexSQL is CREATE [UNIQUE] INDEX, valid on every engine. The index
// is created in the table's schema.
func createIndexSQL(d dialect.Dialect, t dialect.Table, ix dialect.Index) string {
	unique := ""
	if ix.Unique {
		unique = "UNIQUE "
	}
	return "CREATE " + unique + "INDEX " + d.QuoteIdent(ix.Name) + " ON " + qualified(d, t) + " (" + quoteList(d, ix.Columns) + ")"
}

// nullClause returns " NOT NULL" or "".
func nullClause(c dialect.Column) string {
	if c.Nullable {
		return ""
	}
	return " NOT NULL"
}

func typeChanged(from, to dialect.Column) bool {
	return normalizeType(from.Type) != normalizeType(to.Type)
}
