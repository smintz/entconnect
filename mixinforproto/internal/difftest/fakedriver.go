// Package difftest is the driverless differential harness (D-13,
// PIPE-06's first of two independent instances): a real generated
// ent.Client, built against fakeDriver below, proves ent's own
// withHooks pipeline actually invokes mixinforproto's Hooks() entry
// (hooks.go) on a real Save(ctx) call — a property no unit test of
// hooks.go in isolation can show. mixinforproto/internal/boundarytest
// proves the annotation-JSON round trip through a schema-load
// subprocess with no client at all; this package is its sibling, proving
// the mutation-time pipeline instead, still with no database driver in
// mixinforproto's require block (D-13's protected property).
package difftest

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"sync/atomic"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// fakeDriver satisfies entgo.io/ent/dialect.Driver's five methods
// (ExecQuerier's Exec/Query, plus Tx/Close/Dialect) with zero
// dependencies beyond entgo.io/ent itself — already mixinforproto's
// direct dependency (D-13's "no DB driver in mixinforproto's require
// block" property; dialect/sql is ent's own SQL-builder/scan package,
// not a database driver). It never touches a real database:
//
//   - Exec is a no-op — the ResidualCel schema declares no edges, so
//     sqlgraph's creator never opens a transaction (dialect.NopTx wraps
//     the driver directly) and every write this harness performs goes
//     through Query's single-row-scan path below, not Exec.
//   - Query always reports exactly one row containing a fresh,
//     monotonically increasing int64 in a single column, regardless of
//     the query text — enough to satisfy dialect/sql/sqlgraph's
//     insertLastID scan path (an auto-incrementing integer primary key,
//     no edges, no WHERE-predicate-aware behavior needed) for the two
//     mutations tracer_test.go drives. Anything more elaborate is out of
//     scope for this tracer.
type fakeDriver struct {
	dialect string
	nextID  atomic.Int64
}

// newFakeDriver returns a fakeDriver reporting dialectName from
// Dialect() (dialect.SQLite, so dialect/sql/sqlgraph's non-MySQL
// insertLastID branch — the Query-based one — is the one exercised).
func newFakeDriver(dialectName string) *fakeDriver {
	d := &fakeDriver{dialect: dialectName}
	d.nextID.Store(1)
	return d
}

// Exec implements dialect.ExecQuerier. Plan 03-01/03-02's Create-only
// harness never reached this with a real scan target (no edges, no
// MySQL-style LAST_INSERT_ID path — Create's insertLastID goes through
// Query instead, see fakeDriver's own type doc comment). Plan 03-03 adds
// real Update() coverage (hybrid_test.go's D-06/Pitfall 4 tests), and
// dialect/sql/sqlgraph's updateTable unconditionally calls
// res.RowsAffected() on whatever *entsql.Result Exec populates
// (graph.go:1283-1284) — a nil sql.Result interface there panics on the
// method call, not merely returns a wrong count. When v is a
// *entsql.Result, this reports exactly one row affected: every mutation
// this harness drives targets exactly one entity by primary key.
func (d *fakeDriver) Exec(_ context.Context, _ string, _, v any) error {
	if res, ok := v.(*entsql.Result); ok {
		*res = fakeResult{rowsAffected: 1}
	}
	return nil
}

// fakeResult is the database/sql.Result (aliased entsql.Result) this
// harness's Update path needs: dialect/sql/sqlgraph's updateTable calls
// RowsAffected() unconditionally after every non-empty UPDATE statement.
// LastInsertId is never called on this path (that is Create's
// insertLastID, which never reaches Exec for this harness's non-MySQL
// dialect — see fakeDriver.Query's own doc comment) but is implemented
// for completeness and to satisfy the entsql.Result interface.
type fakeResult struct{ rowsAffected int64 }

func (r fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

var _ entsql.Result = fakeResult{}

// Query implements dialect.ExecQuerier. v is always a *entsql.Rows for
// every code path this harness exercises (dialect/sql/sqlgraph's
// insertLastID); a query on a target this fake cannot satisfy is a
// harness bug, not a data condition, so it is reported as an error
// rather than silently returning zero rows.
func (d *fakeDriver) Query(_ context.Context, _ string, _, v any) error {
	rows, ok := v.(*entsql.Rows)
	if !ok {
		return fmt.Errorf("difftest: fakeDriver.Query: unsupported scan target %T (want *entsql.Rows)", v)
	}
	id := d.nextID.Add(1) - 1
	*rows = entsql.Rows{ColumnScanner: &oneInt64Row{value: id}}
	return nil
}

// Tx implements dialect.Driver. Never called by this harness's edge-free
// schema (dialect/sql/sqlgraph's creator uses dialect.NopTx(drv) instead
// whenever hasExternalEdges is false), so this is a named, descriptive
// error rather than a working implementation — a future plan adding
// edges to this harness's schema would need to implement this for real,
// and this error is exactly what would tell it so.
func (d *fakeDriver) Tx(_ context.Context) (dialect.Tx, error) {
	return nil, errors.New("difftest: fakeDriver.Tx: not implemented — not needed for this edge-free, hook-only harness")
}

// Close implements dialect.Driver.
func (d *fakeDriver) Close() error { return nil }

// Dialect implements dialect.Driver.
func (d *fakeDriver) Dialect() string { return d.dialect }

var _ dialect.Driver = (*fakeDriver)(nil)

// oneInt64Row is an entsql.ColumnScanner reporting exactly one row with
// one int64 column — the shape dialect/sql/sqlgraph's insertLastID scan
// path needs for a numeric auto-incrementing ID (dialect/sql's
// ScanInt64/ScanOne, which call Columns/Next/Scan/Err in that order).
type oneInt64Row struct {
	value  int64
	cursor int
}

func (r *oneInt64Row) Close() error { return nil }

func (r *oneInt64Row) ColumnTypes() ([]*stdsql.ColumnType, error) { return nil, nil }

func (r *oneInt64Row) Columns() ([]string, error) { return []string{"id"}, nil }

func (r *oneInt64Row) Err() error { return nil }

func (r *oneInt64Row) Next() bool {
	if r.cursor >= 1 {
		return false
	}
	r.cursor++
	return true
}

func (r *oneInt64Row) NextResultSet() bool { return false }

func (r *oneInt64Row) Scan(dest ...any) error {
	if len(dest) != 1 {
		return fmt.Errorf("difftest: oneInt64Row.Scan: want exactly 1 destination, got %d", len(dest))
	}
	switch d := dest[0].(type) {
	case *int64:
		*d = r.value
		return nil
	case stdsql.Scanner:
		return d.Scan(r.value)
	default:
		return fmt.Errorf("difftest: oneInt64Row.Scan: unsupported destination type %T", dest[0])
	}
}

var _ entsql.ColumnScanner = (*oneInt64Row)(nil)
