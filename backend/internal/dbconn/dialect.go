package dbconn

// Dialect selects explicitly authored SQL. Parameter binding happens at execution.
func (db *Database) Dialect(standard, mysql string) string { return db.driver.Dialect(standard, mysql) }
func (tx *Tx) Dialect(standard, mysql string) string       { return tx.driver.Dialect(standard, mysql) }
func (d Driver) Dialect(standard, mysql string) string {
	if d == MySQL {
		return mysql
	}
	return standard
}
