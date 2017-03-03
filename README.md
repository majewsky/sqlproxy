# go-sql-proxy-driver

This is a driver for the standard library's [`database/sql` package][go-sql]
that passes SQL statements through to another driver, but adds hooks to extend
the standard library's API. The original usecase is to log each executed
statement for debugging purposes.

[go-sql]: https://golang.org/pkg/database/sql/
