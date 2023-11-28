package database

import (
	"fmt"
	"sync"

	"github.com/skeleton1231/gotal/internal/apiserver/store"
	"github.com/skeleton1231/gotal/internal/pkg/errors"
	"github.com/skeleton1231/gotal/internal/pkg/logger"
	"github.com/skeleton1231/gotal/internal/pkg/options"
	"github.com/skeleton1231/gotal/pkg/db"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/fields"
)

type datastore struct {
	db *gorm.DB
	// readerDb *grom.DB
	// writerDb *gorm.DB
}

func (ds *datastore) Close() error {
	db, err := ds.db.DB()
	if err != nil {
		return errors.Wrap(err, "get gorm db instance failed")
	}
	return db.Close()
}

var (
	mysqlFactory store.Factory
	once         sync.Once
)

func GetMySQLFactoryOr(opts *options.MySQLOptions) (store.Factory, error) {
	if opts == nil && mysqlFactory == nil {
		return nil, fmt.Errorf("failed to get mysql store fatory")
	}

	var err error
	var dbIns *gorm.DB
	once.Do(func() {
		options := &db.Options{
			Host:                  opts.Host,
			Username:              opts.Username,
			Password:              opts.Password,
			Database:              opts.Database,
			MaxIdleConnections:    opts.MaxIdleConnections,
			MaxOpenConnections:    opts.MaxOpenConnections,
			MaxConnectionLifeTime: opts.MaxConnectionLifeTime,
			LogLevel:              opts.LogLevel,
			Logger:                logger.New(opts.LogLevel),
		}
		dbIns, err = db.New(options)
		mysqlFactory = &datastore{dbIns}
	})

	if mysqlFactory == nil || err != nil {
		return nil, fmt.Errorf("failed to get mysql store fatory, mysqlFactory: %+v, error: %w", mysqlFactory, err)
	}

	return mysqlFactory, nil
}

// ApplyFieldSelectors applies field selectors to a GORM query.
func ApplyFieldSelectors[T any](db *gorm.DB, model T, fieldSelector string) (*gorm.DB, error) {
	selector, err := fields.ParseSelector(fieldSelector)
	if err != nil {
		return nil, err
	}

	query := db.Model(model)
	for _, requirement := range selector.Requirements() {
		query = query.Where(fmt.Sprintf("%s = ?", requirement.Field), requirement.Value)
	}

	return query, nil
}
