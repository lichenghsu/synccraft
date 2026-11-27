package schema

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/lichenghsu/synccraft/internal/dialect"
)

// Kind is the type of a Change.
type Kind string

const (
	AddTable        Kind = "add_table"
	DropTable       Kind = "drop_table"
	AddColumn       Kind = "add_column"
	DropColumn      Kind = "drop_column"
	AlterColumn     Kind = "alter_column"
	AlterPrimaryKey Kind = "alter_primary_key"
	AddIndex        Kind = "add_index"
	DropIndex       Kind = "drop_index"
)

// Change is one step that moves the target towards the source. Only the
// fields relevant to Kind are set.
type Change struct {
	// ID identifies the change within one comparison: kind, table and the
	// column or index name.
	ID    string        `json:"id"`
	Kind  Kind          `json:"kind"`
	Table dialect.Table `json:"table"`

	// Create is the source table for AddTable.
	Create *Table `json:"create,omitempty"`

	// Column is the source column for AddColumn and AlterColumn, and the
	// target column for DropColumn.
	Column *dialect.Column `json:"column,omitempty"`
	// From is the target column for AlterColumn.
	From *dialect.Column `json:"from,omitempty"`
	// After is the source column that precedes an added column, "" if it is
	// the first one.
	After string `json:"after,omitempty"`

	// Index is the source index for AddIndex, the target index for DropIndex.
	Index *dialect.Index `json:"index,omitempty"`

	// PrimaryKey and FromPrimaryKey are the source and target key columns
	// for AlterPrimaryKey. Either may be empty (no primary key).
	PrimaryKey     []string `json:"primaryKey,omitempty"`
	FromPrimaryKey []string `json:"fromPrimaryKey,omitempty"`

	// Destructive is true when applying the change can lose data.
	Destructive bool `json:"destructive"`
}

// phase orders changes within one table so each step only depends on earlier
// ones: indexes are dropped before the columns they use, columns are added
// before keys and indexes that use them.
var phase = map[Kind]int{
	DropIndex:       0,
	AddColumn:       1,
	AlterColumn:     2,
	AlterPrimaryKey: 3,
	DropColumn:      4,
	AddIndex:        5,
}

// Diff returns the changes that make dst match src, never nil. Tables are
// matched by schema and name, columns and indexes by name. Order: new tables,
// then changes to existing tables (by table, then phase), then dropped tables.
func Diff(src, dst []Table) []Change {
	dstByKey := make(map[dialect.Table]*Table, len(dst))
	for i := range dst {
		dstByKey[dst[i].Table] = &dst[i]
	}
	srcByKey := make(map[dialect.Table]bool, len(src))

	src = sortedTables(src)
	var added, altered, dropped []Change
	for i := range src {
		s := &src[i]
		srcByKey[s.Table] = true
		d, ok := dstByKey[s.Table]
		if !ok {
			added = append(added, Change{Kind: AddTable, Table: s.Table, Create: s})
			continue
		}
		altered = append(altered, diffTable(s, d)...)
	}
	for _, d := range dst {
		if !srcByKey[d.Table] {
			dropped = append(dropped, Change{Kind: DropTable, Table: d.Table, Destructive: true})
		}
	}

	byTable := func(a, b Change) int { return compareTables(a.Table, b.Table) }
	slices.SortStableFunc(added, byTable)
	slices.SortStableFunc(dropped, byTable)
	slices.SortStableFunc(altered, func(a, b Change) int {
		return cmp.Or(compareTables(a.Table, b.Table), cmp.Compare(phase[a.Kind], phase[b.Kind]))
	})

	out := make([]Change, 0, len(added)+len(altered)+len(dropped))
	out = append(out, added...)
	out = append(out, altered...)
	out = append(out, dropped...)
	for i := range out {
		out[i].ID = changeID(out[i])
	}
	return out
}

func changeID(c Change) string {
	id := string(c.Kind) + " " + strconv.Quote(c.Table.Schema) + "." + strconv.Quote(c.Table.Name)
	switch {
	case c.Column != nil:
		id += " " + strconv.Quote(c.Column.Name)
	case c.Index != nil:
		id += " " + strconv.Quote(c.Index.Name)
	}
	return id
}

// diffTable compares two tables with the same key. Changes come out grouped
// by kind; Diff sorts them into phases.
func diffTable(s, d *Table) []Change {
	var out []Change
	t := s.Table

	dstCols := make(map[string]*dialect.Column, len(d.Columns))
	for i := range d.Columns {
		dstCols[d.Columns[i].Name] = &d.Columns[i]
	}
	srcCols := make(map[string]bool, len(s.Columns))
	// Source columns are in Position order, so After always names a column
	// that exists or is added by an earlier change.
	cols := sortedColumns(s.Columns)
	for i := range cols {
		c := &cols[i]
		srcCols[c.Name] = true
		dc, ok := dstCols[c.Name]
		switch {
		case !ok:
			after := ""
			if i > 0 {
				after = cols[i-1].Name
			}
			out = append(out, Change{Kind: AddColumn, Table: t, Column: c, After: after})
		case !columnsEqual(*c, *dc):
			out = append(out, Change{
				Kind: AlterColumn, Table: t, Column: c, From: dc,
				Destructive: normalizeType(c.Type) != normalizeType(dc.Type),
			})
		}
	}
	for i := range d.Columns {
		if !srcCols[d.Columns[i].Name] {
			out = append(out, Change{Kind: DropColumn, Table: t, Column: &d.Columns[i], Destructive: true})
		}
	}

	if !slices.Equal(s.PrimaryKey, d.PrimaryKey) {
		out = append(out, Change{
			Kind: AlterPrimaryKey, Table: t,
			PrimaryKey: s.PrimaryKey, FromPrimaryKey: d.PrimaryKey,
		})
	}

	dstIdx := make(map[string]*dialect.Index, len(d.Indexes))
	for i := range d.Indexes {
		dstIdx[d.Indexes[i].Name] = &d.Indexes[i]
	}
	srcIdx := make(map[string]bool, len(s.Indexes))
	for i := range s.Indexes {
		si := &s.Indexes[i]
		srcIdx[si.Name] = true
		di, ok := dstIdx[si.Name]
		if ok && indexesEqual(*si, *di) {
			continue
		}
		if ok {
			out = append(out, Change{Kind: DropIndex, Table: t, Index: di})
		}
		out = append(out, Change{Kind: AddIndex, Table: t, Index: si})
	}
	for i := range d.Indexes {
		if !srcIdx[d.Indexes[i].Name] {
			out = append(out, Change{Kind: DropIndex, Table: t, Index: &d.Indexes[i]})
		}
	}
	return out
}

func columnsEqual(a, b dialect.Column) bool {
	return normalizeType(a.Type) == normalizeType(b.Type) &&
		a.Nullable == b.Nullable &&
		a.AutoIncrement == b.AutoIncrement &&
		defaultsEqual(a.Default, b.Default)
}

func defaultsEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return normalizeDefault(*a) == normalizeDefault(*b)
}

func indexesEqual(a, b dialect.Index) bool {
	return a.Unique == b.Unique && a.Constraint == b.Constraint && slices.Equal(a.Columns, b.Columns)
}

func compareTables(a, b dialect.Table) int {
	return cmp.Or(cmp.Compare(a.Schema, b.Schema), cmp.Compare(a.Name, b.Name))
}

// sortedTables returns a sorted copy; Diff must not reorder its inputs.
func sortedTables(ts []Table) []Table {
	ts = slices.Clone(ts)
	slices.SortStableFunc(ts, func(a, b Table) int { return compareTables(a.Table, b.Table) })
	return ts
}

func sortedColumns(cs []dialect.Column) []dialect.Column {
	cs = slices.Clone(cs)
	slices.SortStableFunc(cs, func(a, b dialect.Column) int { return cmp.Compare(a.Position, b.Position) })
	return cs
}
