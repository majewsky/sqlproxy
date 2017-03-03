/*******************************************************************************
*
* Copyright 2017 Stefan Majewsky <majewsky@gmx.net>
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

/*

Package sqlproxy provides a database/sql driver that adds hooks to an existing
SQL driver. For example, to augment a PostgreSQL driver with statement logging:

	//this assumes that a "postgresql" driver is already registered
	sql.Register("postgres-with-logging", &sqlproxy.Driver {
		ProxiedDriverName: "postgresql",
		BeforeQueryHook: func(query string, args[]interface{}) {
			log.Printf("SQL: %s %#v", query, args)
		},
	})

There's also a BeforePrepareHook that can be used to reject or edit query
strings.

Caveats

This package is intended for development purposes only, and should not be used
with production databases. It only implements the bare necessities of the
database/sql driver interface and hides optimizations and advanced features of
the proxied SQL driver.

*/
package sqlproxy

import (
	"database/sql"
	"database/sql/driver"
	"io"
)

////////////////////////////////////////////////////////////////////////////////
// driver

//Driver implements sql.Driver. See package documentation for details.
type Driver struct {
	//ProxiedDriverName identifies the SQL driver which will be used to actually
	//perform SQL queries.
	ProxiedDriverName string
	//BeforePrepareHook (optional) runs just before a query is prepared (both for
	//explicit Prepare() calls and one-off queries). The return value will be
	//substituted for the original query string, allowing the hook to rewrite
	//queries arbitrarily. If an error is returned, it will be propagated to the
	//caller of db.Prepare() or tx.Prepare() etc.
	BeforePrepareHook func(query string) (string, error)
	//BeforeQueryHook (optional) runs just before a query is executed, e.g. by
	//the Exec(), Query() or QueryRows() methods of sql.DB, sql.Tx and sql.Stmt.
	BeforeQueryHook func(query string, args []interface{})
}

//Open implements the Driver interface.
func (d *Driver) Open(dataSource string) (driver.Conn, error) {
	db, err := sql.Open(d.ProxiedDriverName, dataSource)
	if err != nil {
		return nil, err
	}
	return &connection{d, db}, nil
}

////////////////////////////////////////////////////////////////////////////////
// connection

type connection struct {
	driver *Driver
	db     *sql.DB
}

//Prepare implements the driver.Conn interface.
func (c *connection) Prepare(query string) (driver.Stmt, error) {
	var err error
	if c.driver.BeforePrepareHook != nil {
		query, err = c.driver.BeforePrepareHook(query)
		if err != nil {
			return nil, err
		}
	}
	stmt, err := c.db.Prepare(query)
	return &statement{c.driver, stmt, query}, err
}

//Close implements the driver.Conn interface.
func (c *connection) Close() error {
	return c.db.Close()
}

//Begin implements the driver.Conn interface.
func (c *connection) Begin() (driver.Tx, error) {
	tx, err := c.db.Begin()
	return tx, err
}

////////////////////////////////////////////////////////////////////////////////
// statement

type statement struct {
	driver *Driver
	stmt   *sql.Stmt
	query  string
}

//Close implements the driver.Stmt interface.
func (s *statement) Close() error {
	return s.stmt.Close()
}

//NumInput implements the driver.Stmt interface.
func (s *statement) NumInput() int {
	//FIXME: the public API of sql.Stmt does not offer that information
	return -1
}

//Exec implements the driver.Stmt interface.
func (s *statement) Exec(values []driver.Value) (driver.Result, error) {
	args := castValues(values)
	s.driver.execBeforeQueryHook(s.query, args)
	return s.stmt.Exec(args)
}

//Query implements the driver.Stmt interface.
func (s *statement) Query(values []driver.Value) (driver.Rows, error) {
	args := castValues(values)
	s.driver.execBeforeQueryHook(s.query, args)
	rows, err := s.stmt.Query(args)
	return &resultRows{rows}, err
}

func (d *Driver) execBeforeQueryHook(query string, args []interface{}) {
	if d.BeforeQueryHook != nil {
		d.BeforeQueryHook(query, args)
	}
}

////////////////////////////////////////////////////////////////////////////////
// rows

type resultRows struct {
	rows *sql.Rows
}

//Columns implements the driver.Rows interface.
func (r *resultRows) Columns() []string {
	result, err := r.rows.Columns()
	if err != nil {
		panic(err)
	}
	return result
}

//Close implements the driver.Rows interface.
func (r *resultRows) Close() error {
	return r.rows.Close()
}

//Next implements the driver.Rows interface.
func (r *resultRows) Next(dest []driver.Value) error {
	if r.rows.Next() {
		return r.rows.Scan(castValues(dest)...)
	}
	return io.EOF
}

////////////////////////////////////////////////////////////////////////////////
// utils

func castValues(values []driver.Value) []interface{} {
	result := make([]interface{}, len(values))
	for idx, arg := range values {
		result[idx] = arg
	}
	return result
}
