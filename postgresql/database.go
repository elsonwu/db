// Copyright (c) 2012-2014 José Carlos Nieto, https://menteslibres.net/xiam
//
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to
// permit persons to whom the Software is furnished to do so, subject to
// the following conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
// LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
// OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
// WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package postgresql

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	// Importing PostgreSQL driver.
	_ "github.com/xiam/gopostgresql"
	"upper.io/cache"
	"upper.io/db"
	"upper.io/db/util/schema"
	"upper.io/db/util/sqlgen"
	"upper.io/db/util/sqlutil"
)

const (
	// Adapter is the public name of the adapter.
	Adapter = `postgresql`
)

var (
	// DateFormat defines the format used for storing dates.
	DateFormat = "2006-01-02 15:04:05.999999999 MST"
	// TimeFormat defines the format used for storing time values.
	TimeFormat = "%d:%02d:%02d.%d"
	// SSLMode defined wheter to enable or disable SSL connections to PostgreSQL
	// server (deprecated).
	SSLMode = false
)

var template *sqlgen.Template

var (
	sqlPlaceholder = sqlgen.Value{sqlgen.Raw{`?`}}
)

type source struct {
	connURL db.ConnectionURL
	session *sql.DB
	tx      *tx
	schema  *schema.DatabaseSchema
}

type columnSchemaT struct {
	Name string `db:"column_name"`
}

func debugEnabled() bool {
	if os.Getenv(db.EnvEnableDebug) != "" {
		return true
	}
	return false
}

func debugLog(query string, args []interface{}, err error, start int64, end int64) {
	if debugEnabled() == true {
		d := sqlutil.Debug{query, args, err, start, end}
		d.Print()
	}
}

func init() {

	template = &sqlgen.Template{
		pgsqlColumnSeparator,
		pgsqlIdentifierSeparator,
		pgsqlIdentifierQuote,
		pgsqlValueSeparator,
		pgsqlValueQuote,
		pgsqlAndKeyword,
		pgsqlOrKeyword,
		pgsqlNotKeyword,
		pgsqlDescKeyword,
		pgsqlAscKeyword,
		pgsqlDefaultOperator,
		pgsqlClauseGroup,
		pgsqlClauseOperator,
		pgsqlColumnValue,
		pgsqlTableAliasLayout,
		pgsqlColumnAliasLayout,
		pgsqlSortByColumnLayout,
		pgsqlWhereLayout,
		pgsqlOrderByLayout,
		pgsqlInsertLayout,
		pgsqlSelectLayout,
		pgsqlUpdateLayout,
		pgsqlDeleteLayout,
		pgsqlTruncateLayout,
		pgsqlDropDatabaseLayout,
		pgsqlDropTableLayout,
		pgsqlSelectCountLayout,
		pgsqlGroupByLayout,
		cache.NewCache(),
	}

	db.Register(Adapter, &source{})
}

func (s *source) populateSchema() (err error) {
	var collections []string

	s.schema = schema.NewDatabaseSchema()

	// Get database name.
	stmt := sqlgen.Statement{
		Type: sqlgen.SqlSelect,
		Columns: sqlgen.Columns{
			{sqlgen.Raw{`CURRENT_DATABASE()`}},
		},
	}

	var row *sql.Row

	if row, err = s.doQueryRow(stmt); err != nil {
		return err
	}

	if err = row.Scan(&s.schema.Name); err != nil {
		return err
	}

	// The Collections() call will populate schema if its nil.
	if collections, err = s.Collections(); err != nil {
		return err
	}

	for i := range collections {
		// Populate each collection.
		if _, err = s.Collection(collections[i]); err != nil {
			return err
		}
	}

	return err
}

func (s *source) doExec(stmt sqlgen.Statement, args ...interface{}) (sql.Result, error) {
	var query string
	var res sql.Result
	var err error
	var start, end int64

	start = time.Now().UnixNano()

	defer func() {
		end = time.Now().UnixNano()
		debugLog(query, args, err, start, end)
	}()

	if s.session == nil {
		return nil, db.ErrNotConnected
	}

	query = stmt.Compile(template)

	l := len(args)
	for i := 0; i < l; i++ {
		query = strings.Replace(query, `?`, fmt.Sprintf(`$%d`, i+1), 1)
	}

	if s.tx != nil {
		res, err = s.tx.sqlTx.Exec(query, args...)
	} else {
		res, err = s.session.Exec(query, args...)
	}

	return res, err
}

func (s *source) doQuery(stmt sqlgen.Statement, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	var query string
	var err error
	var start, end int64

	start = time.Now().UnixNano()

	defer func() {
		end = time.Now().UnixNano()
		debugLog(query, args, err, start, end)
	}()

	if s.session == nil {
		return nil, db.ErrNotConnected
	}

	query = stmt.Compile(template)

	l := len(args)
	for i := 0; i < l; i++ {
		query = strings.Replace(query, `?`, fmt.Sprintf(`$%d`, i+1), 1)
	}

	if s.tx != nil {
		rows, err = s.tx.sqlTx.Query(query, args...)
	} else {
		rows, err = s.session.Query(query, args...)
	}

	return rows, err
}

func (s *source) doQueryRow(stmt sqlgen.Statement, args ...interface{}) (*sql.Row, error) {
	var query string
	var row *sql.Row
	var err error
	var start, end int64

	start = time.Now().UnixNano()

	defer func() {
		end = time.Now().UnixNano()
		debugLog(query, args, err, start, end)
	}()

	if s.session == nil {
		return nil, db.ErrNotConnected
	}

	query = stmt.Compile(template)

	l := len(args)
	for i := 0; i < l; i++ {
		query = strings.Replace(query, `?`, `$`+strconv.Itoa(i+1), 1)
	}

	if s.tx != nil {
		row = s.tx.sqlTx.QueryRow(query, args...)
	} else {
		row = s.session.QueryRow(query, args...)
	}

	return row, err
}

// Returns the string name of the database.
func (s *source) Name() string {
	return s.schema.Name
}

//  Ping verifies a connection to the database is still alive,
//  establishing a connection if necessary.
func (s *source) Ping() error {
	return s.session.Ping()
}

func (s *source) clone() (*source, error) {
	src := new(source)
	src.Setup(s.connURL)

	if err := src.Open(); err != nil {
		return nil, err
	}

	return src, nil
}

func (s *source) Clone() (db.Database, error) {
	return s.clone()
}

func (s *source) Transaction() (db.Tx, error) {
	var err error
	var clone *source
	var sqlTx *sql.Tx

	if sqlTx, err = s.session.Begin(); err != nil {
		return nil, err
	}

	if clone, err = s.clone(); err != nil {
		return nil, err
	}

	tx := &tx{source: clone, sqlTx: sqlTx}

	clone.tx = tx

	return tx, nil
}

// Stores database settings.
func (s *source) Setup(connURL db.ConnectionURL) error {
	s.connURL = connURL
	return s.Open()
}

// Returns the underlying *sql.DB instance.
func (s *source) Driver() interface{} {
	return s.session
}

// Attempts to connect to a database using the stored settings.
func (s *source) Open() error {
	var err error

	// Before db.ConnectionURL we used a unified db.Settings struct. This
	// condition checks for that type and provides backwards compatibility.
	if settings, ok := s.connURL.(db.Settings); ok {

		// User is providing a db.Settings struct, let's translate it into a
		// ConnectionURL{}.
		conn := ConnectionURL{
			User:     settings.User,
			Password: settings.Password,
			Address:  db.HostPort(settings.Host, uint(settings.Port)),
			Database: settings.Database,
			Options: map[string]string{
				"sslmode": "disable",
			},
		}

		// Testing for SSLMode (deprecated)
		if SSLMode {
			conn.Options["sslmode"] = "verify-full"
		}

		// Replace original s.connURL
		s.connURL = conn
	}

	if s.session, err = sql.Open(`postgres`, s.connURL.String()); err != nil {
		return err
	}

	if err = s.populateSchema(); err != nil {
		return err
	}

	return nil
}

// Closes the current database session.
func (s *source) Close() error {
	if s.session != nil {
		return s.session.Close()
	}
	return nil
}

// Changes the active database.
func (s *source) Use(database string) (err error) {
	var conn ConnectionURL

	if conn, err = ParseURL(s.connURL.String()); err != nil {
		return err
	}

	conn.Database = database

	s.connURL = conn

	return s.Open()
}

// Drops the currently active database.
func (s *source) Drop() error {

	_, err := s.doQuery(sqlgen.Statement{
		Type:     sqlgen.SqlDropDatabase,
		Database: sqlgen.Database{s.schema.Name},
	})

	return err
}

// Collections() Returns a list of non-system tables/collections contained
// within the currently active database.
func (s *source) Collections() (collections []string, err error) {

	tablesInSchema := len(s.schema.Tables)

	// Is schema already populated?
	if tablesInSchema > 0 {
		// Pulling table names from schema.
		return s.schema.Tables, nil
	}

	// Schema is empty.

	// Querying table names.
	stmt := sqlgen.Statement{
		Type: sqlgen.SqlSelect,
		Columns: sqlgen.Columns{
			{`table_name`},
		},
		Table: sqlgen.Table{
			`information_schema.tables`,
		},
		Where: sqlgen.Where{
			sqlgen.ColumnValue{
				sqlgen.Column{`table_schema`},
				`=`,
				sqlgen.Value{`public`},
			},
		},
	}

	// Executing statement.
	var rows *sql.Rows
	if rows, err = s.doQuery(stmt); err != nil {
		return nil, err
	}

	defer rows.Close()

	collections = []string{}

	var name string

	for rows.Next() {
		// Getting table name.
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}

		// Adding table entry to schema.
		s.schema.AddTable(name)

		// Adding table to collections array.
		collections = append(collections, name)
	}

	return collections, nil
}

func (s *source) tableExists(names ...string) error {
	var stmt sqlgen.Statement
	var err error
	var rows *sql.Rows

	for i := range names {

		if s.schema.HasTable(names[i]) {
			// We already know this table exists.
			continue
		}

		stmt = sqlgen.Statement{
			Type:  sqlgen.SqlSelect,
			Table: sqlgen.Table{`information_schema.tables`},
			Columns: sqlgen.Columns{
				{`table_name`},
			},
			Where: sqlgen.Where{
				sqlgen.ColumnValue{sqlgen.Column{`table_catalog`}, `=`, sqlPlaceholder},
				sqlgen.ColumnValue{sqlgen.Column{`table_name`}, `=`, sqlPlaceholder},
			},
		}

		if rows, err = s.doQuery(stmt, s.schema.Name, names[i]); err != nil {
			return db.ErrCollectionDoesNotExist
		}

		defer rows.Close()

		if rows.Next() == false {
			return db.ErrCollectionDoesNotExist
		}
	}

	return nil
}

func (s *source) tableColumns(tableName string) ([]string, error) {

	// Making sure this table is allocated.
	tableSchema := s.schema.Table(tableName)

	if len(tableSchema.Columns) > 0 {
		return tableSchema.Columns, nil
	}

	stmt := sqlgen.Statement{
		Type: sqlgen.SqlSelect,
		Table: sqlgen.Table{
			`information_schema.columns`,
		},
		Columns: sqlgen.Columns{
			{`column_name`},
			{`data_type`},
		},
		Where: sqlgen.Where{
			sqlgen.ColumnValue{
				sqlgen.Column{`table_catalog`},
				`=`,
				sqlPlaceholder,
			},
			sqlgen.ColumnValue{
				sqlgen.Column{`table_name`},
				`=`,
				sqlPlaceholder,
			},
		},
	}

	var rows *sql.Rows
	var err error

	if rows, err = s.doQuery(stmt, s.schema.Name, tableName); err != nil {
		return nil, err
	}

	tableFields := []columnSchemaT{}

	if err = sqlutil.FetchRows(rows, &tableFields); err != nil {
		return nil, err
	}

	s.schema.TableInfo[tableName].Columns = make([]string, 0, len(tableFields))

	for i := range tableFields {
		s.schema.TableInfo[tableName].Columns = append(s.schema.TableInfo[tableName].Columns, tableFields[i].Name)
	}

	return s.schema.TableInfo[tableName].Columns, nil
}

// Returns a collection instance by name.
func (s *source) Collection(names ...string) (db.Collection, error) {
	var err error

	if len(names) == 0 {
		return nil, db.ErrMissingCollectionName
	}

	if s.tx != nil {
		if s.tx.done {
			return nil, sql.ErrTxDone
		}
	}

	col := &table{
		source: s,
		names:  names,
	}

	for _, name := range names {
		chunks := strings.SplitN(name, ` `, 2)

		if len(chunks) == 0 {
			return nil, db.ErrMissingCollectionName
		}

		tableName := chunks[0]

		if err := s.tableExists(tableName); err != nil {
			return nil, err
		}

		if col.Columns, err = s.tableColumns(tableName); err != nil {
			return nil, err
		}
	}

	return col, nil
}

func (s *source) getPrimaryKey(tableName string) (string, error) {

	tableSchema := s.schema.Table(tableName)

	if tableSchema.PrimaryKey != "" {
		return tableSchema.PrimaryKey, nil
	}

	// Getting primary key. See https://github.com/upper/db/issues/24.
	stmt := sqlgen.Statement{
		Type:  sqlgen.SqlSelect,
		Table: sqlgen.Table{`pg_index, pg_class, pg_attribute`},
		Columns: sqlgen.Columns{
			{`pg_attribute.attname`},
		},
		Where: sqlgen.Where{
			sqlgen.ColumnValue{sqlgen.Column{`pg_class.oid`}, `=`, sqlgen.Value{sqlgen.Raw{`'"` + tableName + `"'::regclass`}}},
			sqlgen.ColumnValue{sqlgen.Column{`indrelid`}, `=`, sqlgen.Value{sqlgen.Raw{`pg_class.oid`}}},
			sqlgen.ColumnValue{sqlgen.Column{`pg_attribute.attrelid`}, `=`, sqlgen.Value{sqlgen.Raw{`pg_class.oid`}}},
			sqlgen.ColumnValue{sqlgen.Column{`pg_attribute.attnum`}, `=`, sqlgen.Value{sqlgen.Raw{`any(pg_index.indkey)`}}},
			sqlgen.Raw{`indisprimary`},
		},
		Limit: 1,
	}

	var row *sql.Row
	var err error

	if row, err = s.doQueryRow(stmt); err != nil {
		return "", err
	}

	if err = row.Scan(&tableSchema.PrimaryKey); err != nil {
		return "", err
	}

	return tableSchema.PrimaryKey, nil
}
