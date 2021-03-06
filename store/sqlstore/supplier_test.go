// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore_test

import (
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/gorp"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store/sqlstore"
	"github.com/mattermost/mattermost-server/v5/store/storetest"
)

func TestGetReplica(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Description              string
		DataSourceReplicas       []string
		DataSourceSearchReplicas []string
	}{
		{
			"no replicas",
			[]string{},
			[]string{},
		},
		{
			"one source replica",
			[]string{":memory:"},
			[]string{},
		},
		{
			"multiple source replicas",
			[]string{":memory:", ":memory:", ":memory:"},
			[]string{},
		},
		{
			"one source search replica",
			[]string{},
			[]string{":memory:"},
		},
		{
			"multiple source search replicas",
			[]string{},
			[]string{":memory:", ":memory:", ":memory:"},
		},
		{
			"one source replica, one source search replica",
			[]string{":memory:"},
			[]string{":memory:"},
		},
		{
			"one source replica, multiple source search replicas",
			[]string{":memory:"},
			[]string{":memory:", ":memory:", ":memory:"},
		},
		{
			"multiple source replica, one source search replica",
			[]string{":memory:", ":memory:", ":memory:"},
			[]string{":memory:"},
		},
		{
			"multiple source replica, multiple source search replicas",
			[]string{":memory:", ":memory:", ":memory:"},
			[]string{":memory:", ":memory:", ":memory:"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Description+" with license", func(t *testing.T) {
			t.Parallel()

			settings := makeSqlSettings(model.DATABASE_DRIVER_SQLITE)
			settings.DataSourceReplicas = testCase.DataSourceReplicas
			settings.DataSourceSearchReplicas = testCase.DataSourceSearchReplicas
			supplier := sqlstore.NewSqlSupplier(*settings, nil)
			supplier.UpdateLicense(&model.License{})

			replicas := make(map[*gorp.DbMap]bool)
			for i := 0; i < 5; i++ {
				replicas[supplier.GetReplica()] = true
			}

			searchReplicas := make(map[*gorp.DbMap]bool)
			for i := 0; i < 5; i++ {
				searchReplicas[supplier.GetSearchReplica()] = true
			}

			if len(testCase.DataSourceReplicas) > 0 {
				// If replicas were defined, ensure none are the master.
				assert.Len(t, replicas, len(testCase.DataSourceReplicas))

				for replica := range replicas {
					assert.NotEqual(t, supplier.GetMaster(), replica)
				}

			} else if assert.Len(t, replicas, 1) {
				// Otherwise ensure the replicas contains only the master.
				for replica := range replicas {
					assert.Equal(t, supplier.GetMaster(), replica)
				}
			}

			if len(testCase.DataSourceSearchReplicas) > 0 {
				// If search replicas were defined, ensure none are the master nor the replicas.
				assert.Len(t, searchReplicas, len(testCase.DataSourceSearchReplicas))

				for searchReplica := range searchReplicas {
					assert.NotEqual(t, supplier.GetMaster(), searchReplica)
					for replica := range replicas {
						assert.NotEqual(t, searchReplica, replica)
					}
				}

			} else if len(testCase.DataSourceReplicas) > 0 {
				// If no search replicas were defined, but replicas were, ensure they are equal.
				assert.Equal(t, replicas, searchReplicas)

			} else if assert.Len(t, searchReplicas, 1) {
				// Otherwise ensure the search replicas contains the master.
				for searchReplica := range searchReplicas {
					assert.Equal(t, supplier.GetMaster(), searchReplica)
				}
			}
		})

		t.Run(testCase.Description+" without license", func(t *testing.T) {
			t.Parallel()

			settings := makeSqlSettings(model.DATABASE_DRIVER_SQLITE)
			settings.DataSourceReplicas = testCase.DataSourceReplicas
			settings.DataSourceSearchReplicas = testCase.DataSourceSearchReplicas
			supplier := sqlstore.NewSqlSupplier(*settings, nil)

			replicas := make(map[*gorp.DbMap]bool)
			for i := 0; i < 5; i++ {
				replicas[supplier.GetReplica()] = true
			}

			searchReplicas := make(map[*gorp.DbMap]bool)
			for i := 0; i < 5; i++ {
				searchReplicas[supplier.GetSearchReplica()] = true
			}

			if len(testCase.DataSourceReplicas) > 0 {
				// If replicas were defined, ensure none are the master.
				assert.Len(t, replicas, 1)

				for replica := range replicas {
					assert.Same(t, supplier.GetMaster(), replica)
				}

			} else if assert.Len(t, replicas, 1) {
				// Otherwise ensure the replicas contains only the master.
				for replica := range replicas {
					assert.Equal(t, supplier.GetMaster(), replica)
				}
			}

			if len(testCase.DataSourceSearchReplicas) > 0 {
				// If search replicas were defined, ensure none are the master nor the replicas.
				assert.Len(t, searchReplicas, 1)

				for searchReplica := range searchReplicas {
					assert.Same(t, supplier.GetMaster(), searchReplica)
				}

			} else if len(testCase.DataSourceReplicas) > 0 {
				// If no search replicas were defined, but replicas were, ensure they are equal.
				assert.Equal(t, replicas, searchReplicas)

			} else if assert.Len(t, searchReplicas, 1) {
				// Otherwise ensure the search replicas contains the master.
				for searchReplica := range searchReplicas {
					assert.Equal(t, supplier.GetMaster(), searchReplica)
				}
			}
		})
	}
}

func TestGetDbVersion(t *testing.T) {
	testDrivers := []string{
		model.DATABASE_DRIVER_POSTGRES,
		model.DATABASE_DRIVER_MYSQL,
		model.DATABASE_DRIVER_SQLITE,
	}

	for _, driver := range testDrivers {
		t.Run("Should return db version for "+driver, func(t *testing.T) {
			t.Parallel()
			settings := makeSqlSettings(driver)
			supplier := sqlstore.NewSqlSupplier(*settings, nil)

			version, err := supplier.GetDbVersion()
			require.Nil(t, err)
			require.Regexp(t, regexp.MustCompile(`\d+\.\d+(\.\d+)?`), version)
		})
	}
}

func TestRecycleDBConns(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping recycle DBConns test")
	}
	testDrivers := []string{
		model.DATABASE_DRIVER_POSTGRES,
		model.DATABASE_DRIVER_MYSQL,
		model.DATABASE_DRIVER_SQLITE,
	}

	for _, driver := range testDrivers {
		t.Run(driver, func(t *testing.T) {
			settings := makeSqlSettings(driver)
			supplier := sqlstore.NewSqlSupplier(*settings, nil)

			var wg sync.WaitGroup
			tables := []string{"Posts", "Channels", "Users"}
			for _, table := range tables {
				wg.Add(1)
				go func(table string) {
					defer wg.Done()
					query := `SELECT count(*) FROM ` + table
					_, err := supplier.GetMaster().SelectInt(query)
					assert.NoError(t, err)
				}(table)
			}
			wg.Wait()

			stats := supplier.GetMaster().Db.Stats()
			assert.Equal(t, 0, int(stats.MaxLifetimeClosed), "unexpected number of connections closed due to maxlifetime")

			supplier.RecycleDBConnections(2 * time.Second)
			// We cannot reliably control exactly how many open connections are there. So we
			// just do a basic check and confirm that atleast one has been closed.
			stats = supplier.GetMaster().Db.Stats()
			assert.Greater(t, int(stats.MaxLifetimeClosed), 0, "unexpected number of connections closed due to maxlifetime")
		})
	}
}

func TestGetAllConns(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Description              string
		DataSourceReplicas       []string
		DataSourceSearchReplicas []string
		ExpectedNumConnections   int
	}{
		{
			"no replicas",
			[]string{},
			[]string{},
			1,
		},
		{
			"one source replica",
			[]string{":memory:"},
			[]string{},
			2,
		},
		{
			"multiple source replicas",
			[]string{":memory:", ":memory:", ":memory:"},
			[]string{},
			4,
		},
		{
			"one source search replica",
			[]string{},
			[]string{":memory:"},
			1,
		},
		{
			"multiple source search replicas",
			[]string{},
			[]string{":memory:", ":memory:", ":memory:"},
			1,
		},
		{
			"one source replica, one source search replica",
			[]string{":memory:"},
			[]string{":memory:"},
			2,
		},
		{
			"one source replica, multiple source search replicas",
			[]string{":memory:"},
			[]string{":memory:", ":memory:", ":memory:"},
			2,
		},
		{
			"multiple source replica, one source search replica",
			[]string{":memory:", ":memory:", ":memory:"},
			[]string{":memory:"},
			4,
		},
		{
			"multiple source replica, multiple source search replicas",
			[]string{":memory:", ":memory:", ":memory:"},
			[]string{":memory:", ":memory:", ":memory:"},
			4,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Description, func(t *testing.T) {
			t.Parallel()
			settings := makeSqlSettings(model.DATABASE_DRIVER_SQLITE)
			settings.DataSourceReplicas = testCase.DataSourceReplicas
			settings.DataSourceSearchReplicas = testCase.DataSourceSearchReplicas
			supplier := sqlstore.NewSqlSupplier(*settings, nil)

			assert.Len(t, supplier.GetAllConns(), testCase.ExpectedNumConnections)
		})
	}
}

func makeSqlSettings(driver string) *model.SqlSettings {
	switch driver {
	case model.DATABASE_DRIVER_POSTGRES:
		return storetest.MakeSqlSettings(driver)
	case model.DATABASE_DRIVER_MYSQL:
		return storetest.MakeSqlSettings(driver)
	case model.DATABASE_DRIVER_SQLITE:
		return makeSqliteSettings()
	}

	return nil
}

func makeSqliteSettings() *model.SqlSettings {
	driverName := model.DATABASE_DRIVER_SQLITE
	dataSource := ":memory:"
	maxIdleConns := 1
	connMaxLifetimeMilliseconds := 3600000
	maxOpenConns := 1
	queryTimeout := 5

	return &model.SqlSettings{
		DriverName:                  &driverName,
		DataSource:                  &dataSource,
		MaxIdleConns:                &maxIdleConns,
		ConnMaxLifetimeMilliseconds: &connMaxLifetimeMilliseconds,
		MaxOpenConns:                &maxOpenConns,
		QueryTimeout:                &queryTimeout,
	}
}
