package dbwork

import (
	"context"
	"database/sql"
	"database/sql/driver"
)

// splitConnector owns both pool lifetimes. database/sql calls its Close when
// the public handle closes, so tests and failed startup cannot leak writers.
type splitConnector struct {
	dsn    string
	driver *splitDriver
}
type splitDriver struct {
	driver.Driver
	writer     *sql.DB
	background *sql.DB
}

func (c *splitConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.driver.Open(c.dsn)
}
func (c *splitConnector) Driver() driver.Driver { return c.driver }
func (c *splitConnector) Close() error {
	backgroundErr := c.driver.background.Close()
	writerErr := c.driver.writer.Close()
	if backgroundErr != nil {
		return backgroundErr
	}
	return writerErr
}

func writerHandle(db *sql.DB) *sql.DB {
	if d, ok := db.Driver().(*splitDriver); ok {
		return d.writer
	}
	return db // Handles supplied by embedders still use their own policy.
}

// ReadHandle selects the two-connection background read pool for classified
// background work. The public handle remains the six-connection foreground
// pool; neither can borrow the other's read capacity.
func ReadHandle(ctx context.Context, db *sql.DB) *sql.DB {
	if db == nil {
		return nil
	}
	if d, ok := db.Driver().(*splitDriver); ok && ClassFrom(ctx, ClassInteractive).Background() {
		return d.background
	}
	return db
}
