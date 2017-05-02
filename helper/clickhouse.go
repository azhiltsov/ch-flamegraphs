package helper

import (
	"time"
	"database/sql"

	_ "github.com/kshvakov/clickhouse"

	"github.com/kshvakov/clickhouse"
)

func DBStartTransaction(db *sql.DB, query string) (*sql.Tx, *sql.Stmt, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}

	stmt, err := tx.Prepare(query)
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}

	return tx, stmt, nil
}

type ClickhouseSender struct{
	db *sql.DB
	tx *sql.Tx
	stmt *sql.Stmt
	linesToBuffer int
	lines int
	commitedLines int64
	version int64
	now time.Time
	txStart time.Time
}

func NewClickhouseSender(db *sql.DB, query string, t int64, rowsPerInsert int) (*ClickhouseSender, error) {
	tx, stmt, err := DBStartTransaction(db, query)
	if err != nil {
		return nil, err
	}
	return &ClickhouseSender{
		db: db,
		tx: tx,
		stmt: stmt,
		version: t,
		now: time.Now(),
		txStart: time.Now(),
		linesToBuffer: rowsPerInsert,
	}, nil
}

func (c *ClickhouseSender) SendFg(cluster, name string, id uint64, mtime int64, total, value, parentID uint64, childrenIds []uint64, level uint64) error {
	c.lines++
	_, err := c.stmt.Exec(
		c.version,
		"graphite_metrics",
		cluster,
		id,
		name,
		total,
		value,
		parentID,
		clickhouse.Array(childrenIds),
		level,
		mtime,
		c.now,
		c.version,
	)
	if err != nil {
		return err
	}

	if c.lines >= c.linesToBuffer || time.Since(c.txStart) > 280 * time.Second {
		err = c.tx.Commit()
		if err != nil {
			return err
		}
		c.tx, c.stmt, err = DBStartTransaction(c.db,"INSERT INTO flamegraph (timestamp, graph_type, cluster, id, name, total, value, parent_id, children_ids, level, mtime, date, version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			return err
		}
		c.commitedLines += int64(c.lines)
		c.lines = 0
		c.txStart = time.Now()
	}

	return err
}

func (c *ClickhouseSender) SendMetricStats(cluster, path string, id uint64, mtime, atime, rdtime, count int64) error {
	c.lines++
	_, err := c.stmt.Exec(
		c.version,
		"graphite_metrics",
		cluster,
		id,
		path,
		mtime,
		atime,
		rdtime,
		count,
		c.now,
		c.version,
	)
	if err != nil {
		return err
	}

	if c.lines >= c.linesToBuffer || time.Since(c.txStart) > 280 * time.Second {
		err = c.tx.Commit()
		if err != nil {
			return err
		}
		c.tx, c.stmt, err = DBStartTransaction(c.db,"INSERT INTO metricstats (timestamp, graph_type, cluster, id, name, mtime, atime, rdtime, count, date, version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		if err != nil {
			return err
		}
		c.commitedLines += int64(c.lines)
		c.lines = 0
		c.txStart = time.Now()
	}

	return err
}

func (c *ClickhouseSender) Commit() (int64, error) {
	c.commitedLines += int64(c.lines)
	return c.commitedLines, c.tx.Commit()
}
