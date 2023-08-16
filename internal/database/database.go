package database

import (
	"context"
	"database/sql"
	"reflect"
	"time"

	_ "github.com/zitadel/zitadel/internal/database/cockroach"
	"github.com/zitadel/zitadel/internal/database/dialect"
	_ "github.com/zitadel/zitadel/internal/database/postgres"
	"github.com/zitadel/zitadel/internal/errors"
)

type Config struct {
	Dialects  map[string]interface{} `mapstructure:",remain"`
	connector dialect.Connector
}

func (c *Config) SetConnector(connector dialect.Connector) {
	c.connector = connector
}

type DB struct {
	*sql.DB
	dialect.Database
	queryCommitDelay time.Duration
}

func (db *DB) SetQueryCommitDelay(d time.Duration) {
	db.queryCommitDelay = d
}

func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(context.Background(), query, args...)
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() {
		if db.queryCommitDelay == 0 {
			_ = tx.Commit()
			return
		}
		go func() {
			time.Sleep(db.queryCommitDelay)
			_ = tx.Commit()
		}()
	}()
	return tx.Query(query, args...)
}
func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		// because we need to return [*sql.Row] we have to call the function
		return db.DB.QueryRowContext(ctx, query, args...)
	}
	defer func() {
		if db.queryCommitDelay == 0 {
			_ = tx.Commit()
			return
		}
		go func() {
			time.Sleep(db.queryCommitDelay)
			_ = tx.Commit()
		}()
	}()
	return tx.QueryRowContext(ctx, query, args...)
}

func Connect(config Config, useAdmin bool) (*DB, error) {
	client, err := config.connector.Connect(useAdmin)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(); err != nil {
		return nil, errors.ThrowPreconditionFailed(err, "DATAB-0pIWD", "Errors.Database.Connection.Failed")
	}

	return &DB{
		DB:               client,
		Database:         config.connector,
		queryCommitDelay: 10 * time.Millisecond,
	}, nil
}

func DecodeHook(from, to reflect.Value) (interface{}, error) {
	if to.Type() != reflect.TypeOf(Config{}) {
		return from.Interface(), nil
	}

	configuredDialects, ok := from.Interface().(map[string]interface{})
	if !ok {
		return from.Interface(), nil
	}

	configuredDialect := dialect.SelectByConfig(configuredDialects)
	configs := make([]interface{}, 0, len(configuredDialects)-1)

	for name, dialectConfig := range configuredDialects {
		if !configuredDialect.Matcher.MatchName(name) {
			continue
		}

		configs = append(configs, dialectConfig)
	}

	connector, err := configuredDialect.Matcher.Decode(configs)
	if err != nil {
		return nil, err
	}

	return Config{connector: connector}, nil
}

func (c Config) DatabaseName() string {
	return c.connector.DatabaseName()
}

func (c Config) Username() string {
	return c.connector.Username()
}

func (c Config) Password() string {
	return c.connector.Password()
}

func (c Config) Type() string {
	return c.connector.Type()
}
