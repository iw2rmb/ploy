// Package testutils provides database utilities for testing
package testutils

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// TestDatabase provides utilities for database testing
type TestDatabase struct {
	DB     *sql.DB
	config *TestConfig
}

// SetupTestDB creates a test database for integration tests
func SetupTestDB(t *testing.T) *TestDatabase {
	t.Helper()

	config := DefaultTestConfig()

	// Connect to PostgreSQL
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		config.PostgresUser,
		config.PostgresPassword,
		config.PostgresHost,
		config.PostgresPort,
		config.PostgresDatabase)

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err, "Failed to connect to test database")

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	require.NoError(t, err, "Failed to ping test database")

	testDB := &TestDatabase{
		DB:     db,
		config: config,
	}

	// Run migrations if available
	testDB.runMigrations(t)

	// Setup cleanup
	t.Cleanup(func() {
		testDB.Cleanup(t)
	})

	return testDB
}

// runMigrations runs database migrations for testing
func (tdb *TestDatabase) runMigrations(t *testing.T) {
	t.Helper()

	// Check if migrations directory exists
	migrationsPath := "file://migrations"
	if !filepath.IsAbs(migrationsPath) {
		// Try to find migrations directory
		possiblePaths := []string{
			"file://migrations",
			"file://../../migrations",
			"file://../../../migrations",
			"file://../../../../migrations",
		}

		migrationsPath = possiblePaths[0] // Default fallback
		for _, path := range possiblePaths {
			if _, err := filepath.Glob(filepath.Join(filepath.Clean(path[7:]), "*.sql")); err == nil {
				migrationsPath = path
				break
			}
		}
	}

	// Create migration driver
	driver, err := postgres.WithInstance(tdb.DB, &postgres.Config{})
	if err != nil {
		t.Logf("Failed to create migration driver: %v", err)
		return // Don't fail test if migrations aren't available
	}

	m, err := migrate.NewWithDatabaseInstance(
		migrationsPath,
		"postgres", driver)
	if err != nil {
		t.Logf("Failed to create migrator: %v", err)
		return // Don't fail test if migrations aren't available
	}
	defer m.Close()

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Logf("Failed to run migrations: %v", err)
		// Don't fail test - migrations might not be available
	}
}

// Cleanup removes all test data and closes the database connection
func (tdb *TestDatabase) Cleanup(t *testing.T) {
	t.Helper()

	// Clean up test data
	tdb.TruncateAllTables(t)

	// Close database connection
	if tdb.DB != nil {
		err := tdb.DB.Close()
		if err != nil {
			t.Logf("Failed to close database connection: %v", err)
		}
	}
}

// TruncateAllTables removes all data from test tables
func (tdb *TestDatabase) TruncateAllTables(t *testing.T) {
	t.Helper()

	// Common table names that might exist in Ploy
	tables := []string{
		"benchmarks",
		"recipes",
		"sandboxes",
		"transformations",
		"applications",
		"deployments",
		"build_logs",
		"certificates",
		"domains",
		"artifacts",
		"health_checks",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Disable foreign key checks temporarily
	_, err := tdb.DB.ExecContext(ctx, "SET session_replication_role = replica;")
	if err != nil {
		t.Logf("Failed to disable foreign key checks: %v", err)
	}

	for _, table := range tables {
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
		_, err := tdb.DB.ExecContext(ctx, query)
		if err != nil {
			// Table might not exist, which is fine for testing
			t.Logf("Failed to truncate table %s: %v", table, err)
		}
	}

	// Re-enable foreign key checks
	_, err = tdb.DB.ExecContext(ctx, "SET session_replication_role = DEFAULT;")
	if err != nil {
		t.Logf("Failed to re-enable foreign key checks: %v", err)
	}
}

// SeedTestData populates database with test data
func (tdb *TestDatabase) SeedTestData(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Seed common test data
	tdb.seedRecipes(t, ctx)
	tdb.seedApplications(t, ctx)
	tdb.seedDomains(t, ctx)
}

// seedRecipes inserts test recipe data
func (tdb *TestDatabase) seedRecipes(t *testing.T, ctx context.Context) {
	t.Helper()

	query := `
		INSERT INTO recipes (id, name, language, category, description, created_at, updated_at)
		VALUES 
			($1, $2, $3, $4, $5, $6, $7),
			($8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO NOTHING`

	now := time.Now()
	_, err := tdb.DB.ExecContext(ctx, query,
		"test-recipe-1", "Test Java Recipe", "java", "cleanup", "Test recipe for Java cleanup", now, now,
		"test-recipe-2", "Test Go Recipe", "go", "security", "Test recipe for Go security", now, now,
	)
	if err != nil {
		t.Logf("Failed to seed recipes (table might not exist): %v", err)
	}
}

// seedApplications inserts test application data
func (tdb *TestDatabase) seedApplications(t *testing.T, ctx context.Context) {
	t.Helper()

	query := `
		INSERT INTO applications (id, name, language, lane, status, created_at, updated_at)
		VALUES 
			($1, $2, $3, $4, $5, $6, $7),
			($8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO NOTHING`

	now := time.Now()
	_, err := tdb.DB.ExecContext(ctx, query,
		"test-app-1", "test-go-app", "go", "B", "deployed", now, now,
		"test-app-2", "test-node-app", "javascript", "E", "building", now, now,
	)
	if err != nil {
		t.Logf("Failed to seed applications (table might not exist): %v", err)
	}
}

// seedDomains inserts test domain data
func (tdb *TestDatabase) seedDomains(t *testing.T, ctx context.Context) {
	t.Helper()

	query := `
		INSERT INTO domains (id, name, app_id, status, created_at, updated_at)
		VALUES 
			($1, $2, $3, $4, $5, $6),
			($7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO NOTHING`

	now := time.Now()
	_, err := tdb.DB.ExecContext(ctx, query,
		"test-domain-1", "test-go-app.local.dev", "test-app-1", "active", now, now,
		"test-domain-2", "test-node-app.local.dev", "test-app-2", "pending", now, now,
	)
	if err != nil {
		t.Logf("Failed to seed domains (table might not exist): %v", err)
	}
}

// WithTransaction runs a function within a database transaction
func (tdb *TestDatabase) WithTransaction(t *testing.T, fn func(*sql.Tx)) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := tdb.DB.BeginTx(ctx, nil)
	require.NoError(t, err, "Failed to begin transaction")

	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			t.Logf("Failed to rollback transaction: %v", err)
		}
	}()

	fn(tx)
}

// ExecuteQuery executes a query and returns the result
func (tdb *TestDatabase) ExecuteQuery(t *testing.T, query string, args ...interface{}) *sql.Rows {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := tdb.DB.QueryContext(ctx, query, args...)
	require.NoError(t, err, "Failed to execute query: %s", query)

	return rows
}

// ExecuteNonQuery executes a non-query statement
func (tdb *TestDatabase) ExecuteNonQuery(t *testing.T, query string, args ...interface{}) sql.Result {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := tdb.DB.ExecContext(ctx, query, args...)
	require.NoError(t, err, "Failed to execute non-query: %s", query)

	return result
}

// CountRows counts rows in a table with optional WHERE clause
func (tdb *TestDatabase) CountRows(t *testing.T, table string, whereClause string, args ...interface{}) int {
	t.Helper()

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	err := tdb.DB.QueryRowContext(ctx, query, args...).Scan(&count)
	require.NoError(t, err, "Failed to count rows in table: %s", table)

	return count
}

// TableExists checks if a table exists in the database
func (tdb *TestDatabase) TableExists(t *testing.T, tableName string) bool {
	t.Helper()

	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		)`

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := tdb.DB.QueryRowContext(ctx, query, tableName).Scan(&exists)
	if err != nil {
		t.Logf("Failed to check table existence: %v", err)
		return false
	}

	return exists
}

// WaitForTable waits for a table to be created (useful for migration testing)
func (tdb *TestDatabase) WaitForTable(t *testing.T, tableName string, timeout time.Duration) {
	t.Helper()

	AssertEventually(t, func() bool {
		return tdb.TableExists(t, tableName)
	}, timeout, fmt.Sprintf("Table %s was not created within timeout", tableName))
}

// DatabaseTestSuite provides a reusable test suite for database operations
type DatabaseTestSuite struct {
	DB *TestDatabase
	t  *testing.T
}

// NewDatabaseTestSuite creates a new database test suite
func NewDatabaseTestSuite(t *testing.T) *DatabaseTestSuite {
	return &DatabaseTestSuite{
		DB: SetupTestDB(t),
		t:  t,
	}
}

// BeforeTest runs before each test method
func (suite *DatabaseTestSuite) BeforeTest() {
	suite.t.Helper()
	suite.DB.TruncateAllTables(suite.t)
	suite.DB.SeedTestData(suite.t)
}

// AfterTest runs after each test method
func (suite *DatabaseTestSuite) AfterTest() {
	suite.t.Helper()
	suite.DB.TruncateAllTables(suite.t)
}

// InsertTestRecord inserts a test record and returns the ID
func (suite *DatabaseTestSuite) InsertTestRecord(table string, data map[string]interface{}) int64 {
	suite.t.Helper()

	var columns []string
	var placeholders []string
	var values []interface{}

	i := 1
	for column, value := range data {
		columns = append(columns, column)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, value)
		i++
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING id",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var id int64
	err := suite.DB.DB.QueryRowContext(ctx, query, values...).Scan(&id)
	require.NoError(suite.t, err, "Failed to insert test record")

	return id
}

// AssertRecordExists asserts that a record exists with given conditions
func (suite *DatabaseTestSuite) AssertRecordExists(table string, conditions map[string]interface{}) {
	suite.t.Helper()

	var whereParts []string
	var values []interface{}

	i := 1
	for column, value := range conditions {
		whereParts = append(whereParts, fmt.Sprintf("%s = $%d", column, i))
		values = append(values, value)
		i++
	}

	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM %s WHERE %s",
		table,
		strings.Join(whereParts, " AND "),
	)

	count := suite.DB.CountRows(suite.t, table, strings.Join(whereParts, " AND "), values...)
	require.Greater(suite.t, count, 0, "Expected record to exist in table %s", table)
}

// AssertRecordNotExists asserts that no record exists with given conditions
func (suite *DatabaseTestSuite) AssertRecordNotExists(table string, conditions map[string]interface{}) {
	suite.t.Helper()

	var whereParts []string
	var values []interface{}

	i := 1
	for column, value := range conditions {
		whereParts = append(whereParts, fmt.Sprintf("%s = $%d", column, i))
		values = append(values, value)
		i++
	}

	count := suite.DB.CountRows(suite.t, table, strings.Join(whereParts, " AND "), values...)
	require.Equal(suite.t, 0, count, "Expected no record to exist in table %s", table)
}