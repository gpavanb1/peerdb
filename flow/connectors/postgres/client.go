package connpostgres

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PeerDB-io/peer-flow/connectors/utils"
	"github.com/PeerDB-io/peer-flow/generated/protos"
	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
)

//nolint:stylecheck
const (
	internalSchema            = "_peerdb_internal"
	mirrorJobsTableIdentifier = "peerdb_mirror_jobs"
	createMirrorJobsTableSQL  = `CREATE TABLE IF NOT EXISTS %s.%s(mirror_job_name TEXT PRIMARY KEY,
		lsn_offset BIGINT NOT NULL,sync_batch_id BIGINT NOT NULL,normalize_batch_id BIGINT NOT NULL)`
	rawTablePrefix          = "_peerdb_raw"
	createInternalSchemaSQL = "CREATE SCHEMA IF NOT EXISTS %s"
	createRawTableSQL       = `CREATE TABLE IF NOT EXISTS %s.%s(_peerdb_uid TEXT NOT NULL,
		_peerdb_timestamp BIGINT NOT NULL,_peerdb_destination_table_name TEXT NOT NULL,_peerdb_data JSONB NOT NULL,
		_peerdb_record_type INTEGER NOT NULL, _peerdb_match_data JSONB,_peerdb_batch_id INTEGER,
		_peerdb_unchanged_toast_columns TEXT)`

	getLastOffsetSQL            = "SELECT lsn_offset FROM %s.%s WHERE mirror_job_name=$1"
	getLastSyncBatchID_SQL      = "SELECT sync_batch_id FROM %s.%s WHERE mirror_job_name=$1"
	getLastNormalizeBatchID_SQL = "SELECT normalize_batch_id FROM %s.%s WHERE mirror_job_name=$1"
	createNormalizedTableSQL    = "CREATE TABLE IF NOT EXISTS %s(%s)"

	insertJobMetadataSQL                 = "INSERT INTO %s.%s VALUES ($1,$2,$3,$4)"
	checkIfJobMetadataExistsSQL          = "SELECT COUNT(1)::TEXT::BOOL FROM %s.%s WHERE mirror_job_name=$1"
	updateMetadataForSyncRecordsSQL      = "UPDATE %s.%s SET lsn_offset=$1, sync_batch_id=$2 WHERE mirror_job_name=$3"
	updateMetadataForNormalizeRecordsSQL = "UPDATE %s.%s SET normalize_batch_id=$1 WHERE mirror_job_name=$2"

	getTableNameToUnchangedToastColsSQL = `SELECT _peerdb_destination_table_name,
	ARRAY_AGG(DISTINCT _peerdb_unchanged_toast_columns) FROM %s.%s WHERE
	_peerdb_batch_id>$1 AND _peerdb_batch_id<=$2 GROUP BY _peerdb_destination_table_name`
	srcTableName      = "src"
	mergeStatementSQL = `WITH src_rank AS (
		SELECT _peerdb_data,_peerdb_record_type,_peerdb_unchanged_toast_columns,
		RANK() OVER (PARTITION BY %s ORDER BY _peerdb_timestamp DESC) AS rank
		FROM %s.%s WHERE _peerdb_batch_id>$1 AND _peerdb_batch_id<=$2 AND _peerdb_destination_table_name=$3 
	)
	MERGE INTO %s dst
	USING (SELECT %s,_peerdb_record_type,_peerdb_unchanged_toast_columns FROM src_rank WHERE rank=1) src
	ON dst.%s=src.%s
	WHEN NOT MATCHED THEN
	INSERT (%s) VALUES (%s)
	%s
	WHEN MATCHED AND src._peerdb_record_type=2 THEN
	DELETE`

	dropTableIfExistsSQL = "DROP TABLE IF EXISTS %s.%s"
	deleteJobMetadataSQL = "DELETE FROM %s.%s WHERE MIRROR_JOB_NAME=?"
)

// getRelIDForTable returns the relation ID for a table.
func (c *PostgresConnector) getRelIDForTable(schemaTable *SchemaTable) (uint32, error) {
	var relID uint32
	err := c.pool.QueryRow(c.ctx,
		`SELECT c.oid FROM pg_class c JOIN pg_namespace n
		 ON n.oid = c.relnamespace WHERE n.nspname = $1 AND c.relname = $2`,
		strings.ToLower(schemaTable.Schema), strings.ToLower(schemaTable.Table)).Scan(&relID)
	if err != nil {
		return 0, fmt.Errorf("error getting relation ID for table %s: %w", schemaTable, err)
	}

	return relID, nil
}

// getPrimaryKeyColumn for table returns the primary key column for a given table
// errors if there is no primary key column or if there is more than one primary key column.
func (c *PostgresConnector) getPrimaryKeyColumn(schemaTable *SchemaTable) (string, error) {
	relID, err := c.getRelIDForTable(schemaTable)
	if err != nil {
		return "", fmt.Errorf("failed to get relation id for table %s: %w", schemaTable, err)
	}

	// Get the primary key column name
	var pkCol string
	rows, err := c.pool.Query(c.ctx,
		`SELECT a.attname FROM pg_index i
		 JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		 WHERE i.indrelid = $1 AND i.indisprimary`,
		relID)
	if err != nil {
		return "", fmt.Errorf("error getting primary key column for table %s: %w", schemaTable, err)
	}
	defer rows.Close()
	// 0 rows returned, table has no primary keys
	if !rows.Next() {
		return "", fmt.Errorf("table %s has no primary keys", schemaTable)
	}
	err = rows.Scan(&pkCol)
	if err != nil {
		return "", fmt.Errorf("error scanning primary key column for table %s: %w", schemaTable, err)
	}
	// more than 1 row returned, table has more than 1 primary key
	if rows.Next() {
		return "", fmt.Errorf("table %s has more than one primary key", schemaTable)
	}

	return pkCol, nil
}

func (c *PostgresConnector) tableExists(schemaTable *SchemaTable) (bool, error) {
	var exists bool
	err := c.pool.QueryRow(c.ctx,
		`SELECT EXISTS (
			SELECT FROM pg_tables
			WHERE schemaname = $1
			AND tablename = $2
		)`,
		schemaTable.Schema,
		schemaTable.Table,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking if table exists: %w", err)
	}

	return exists, nil
}

// checkSlotAndPublication checks if the replication slot and publication exist.
func (c *PostgresConnector) checkSlotAndPublication(slot string, publication string) (*SlotCheckResult, error) {
	slotExists := false
	publicationExists := false

	// Check if the replication slot exists
	var slotName string
	err := c.pool.QueryRow(c.ctx,
		"SELECT slot_name FROM pg_replication_slots WHERE slot_name = $1",
		slot).Scan(&slotName)
	if err != nil {
		// check if the error is a "no rows" error
		if err != pgx.ErrNoRows {
			return nil, fmt.Errorf("error checking for replication slot - %s: %w", slot, err)
		}
	} else {
		slotExists = true
	}

	// Check if the publication exists
	var pubName string
	err = c.pool.QueryRow(c.ctx,
		"SELECT pubname FROM pg_publication WHERE pubname = $1",
		publication).Scan(&pubName)
	if err != nil {
		// check if the error is a "no rows" error
		if err != pgx.ErrNoRows {
			return nil, fmt.Errorf("error checking for publication - %s: %w", publication, err)
		}
	} else {
		publicationExists = true
	}

	return &SlotCheckResult{
		SlotExists:        slotExists,
		PublicationExists: publicationExists,
	}, nil
}

// createSlotAndPublication creates the replication slot and publication.
func (c *PostgresConnector) createSlotAndPublication(
	s *SlotCheckResult,
	slot string,
	publication string,
	tableNameMapping map[string]string,
) error {
	/*
		iterating through source tables and creating a publication.
		expecting tablenames to be schema qualified
	*/
	srcTableNames := make([]string, 0, len(tableNameMapping))
	for srcTableName := range tableNameMapping {
		if len(strings.Split(srcTableName, ".")) != 2 {
			return fmt.Errorf("source tables identifier is invalid: %v", srcTableName)
		}
		srcTableNames = append(srcTableNames, srcTableName)
	}
	tableNameString := strings.Join(srcTableNames, ", ")

	if !s.PublicationExists {
		// Create the publication to help filter changes only for the given tables
		stmt := fmt.Sprintf("CREATE PUBLICATION %s FOR TABLE %s", publication, tableNameString)
		_, err := c.pool.Exec(c.ctx, stmt)
		if err != nil {
			return fmt.Errorf("error creating publication: %w", err)
		}
	}

	// create slot only after we succeeded in creating publication.
	if !s.SlotExists {
		// Create the logical replication slot
		_, err := c.pool.Exec(c.ctx,
			"SELECT * FROM pg_create_logical_replication_slot($1, 'pgoutput')",
			slot)
		if err != nil {
			return fmt.Errorf("error creating replication slot: %w", err)
		}
	}

	return nil
}

func (c *PostgresConnector) createInternalSchema(createSchemaTx pgx.Tx) error {
	_, err := createSchemaTx.Exec(c.ctx, fmt.Sprintf(createInternalSchemaSQL, internalSchema))
	if err != nil {
		return fmt.Errorf("error while creating internal schema: %w", err)
	}
	return nil
}

func getRawTableIdentifier(jobName string) string {
	jobName = regexp.MustCompile("[^a-zA-Z0-9]+").ReplaceAllString(jobName, "_")
	return fmt.Sprintf("%s_%s", rawTablePrefix, strings.ToLower(jobName))
}

func generateCreateTableSQLForNormalizedTable(sourceTableIdentifier string,
	sourceTableSchema *protos.TableSchema) string {
	createTableSQLArray := make([]string, 0, len(sourceTableSchema.Columns))
	for columnName, genericColumnType := range sourceTableSchema.Columns {
		if sourceTableSchema.PrimaryKeyColumn == strings.ToLower(columnName) {
			createTableSQLArray = append(createTableSQLArray, fmt.Sprintf("%s %s PRIMARY KEY,",
				columnName, qValueKindToPostgresType(genericColumnType)))
		} else {
			createTableSQLArray = append(createTableSQLArray, fmt.Sprintf("%s %s,", columnName,
				qValueKindToPostgresType(genericColumnType)))
		}
	}
	return fmt.Sprintf(createNormalizedTableSQL, sourceTableIdentifier,
		strings.TrimSuffix(strings.Join(createTableSQLArray, ""), ","))
}

func (c *PostgresConnector) getLastSyncBatchID(jobName string) (int64, error) {
	rows, err := c.pool.Query(c.ctx, fmt.Sprintf(getLastSyncBatchID_SQL, internalSchema,
		mirrorJobsTableIdentifier), jobName)
	if err != nil {
		return 0, fmt.Errorf("error querying Postgres peer for last syncBatchId: %w", err)
	}
	defer rows.Close()

	var result int64
	if !rows.Next() {
		log.Warnf("No row found for job %s, returning 0", jobName)
		return 0, nil
	}
	err = rows.Scan(&result)
	if err != nil {
		return 0, fmt.Errorf("error while reading result row: %w", err)
	}
	return result, nil
}

func (c *PostgresConnector) getLastNormalizeBatchID(jobName string) (int64, error) {
	rows, err := c.pool.Query(c.ctx, fmt.Sprintf(getLastNormalizeBatchID_SQL, internalSchema,
		mirrorJobsTableIdentifier), jobName)
	if err != nil {
		return 0, fmt.Errorf("error querying Postgres peer for last normalizeBatchId: %w", err)
	}
	defer rows.Close()

	var result int64
	if !rows.Next() {
		log.Warnf("No row found for job %s, returning 0", jobName)
		return 0, nil
	}
	err = rows.Scan(&result)
	if err != nil {
		return 0, fmt.Errorf("error while reading result row: %w", err)
	}
	return result, nil
}

func (c *PostgresConnector) jobMetadataExists(jobName string) (bool, error) {
	rows, err := c.pool.Query(c.ctx,
		fmt.Sprintf(checkIfJobMetadataExistsSQL, internalSchema, mirrorJobsTableIdentifier), jobName)
	if err != nil {
		return false, fmt.Errorf("failed to check if job exists: %w", err)
	}
	defer rows.Close()

	var result bool
	rows.Next()
	err = rows.Scan(&result)
	if err != nil {
		return false, fmt.Errorf("error reading result row: %w", err)
	}
	return result, nil
}

func (c *PostgresConnector) majorVersionCheck(majorVersion int) (bool, error) {
	var version int
	err := c.pool.QueryRow(c.ctx, "SELECT current_setting('server_version_num')::INTEGER").Scan(&version)
	if err != nil {
		return false, fmt.Errorf("failed to get server version: %w", err)
	}

	return version >= majorVersion, nil
}

func (c *PostgresConnector) updateSyncMetadata(flowJobName string, lastCP int64, syncBatchID int64,
	syncRecordsTx pgx.Tx) error {
	jobMetadataExists, err := c.jobMetadataExists(flowJobName)
	if err != nil {
		return fmt.Errorf("failed to get sync status for flow job: %w", err)
	}

	if !jobMetadataExists {
		_, err := syncRecordsTx.Exec(c.ctx,
			fmt.Sprintf(insertJobMetadataSQL, internalSchema, mirrorJobsTableIdentifier),
			flowJobName, lastCP, syncBatchID, 0)
		if err != nil {
			return fmt.Errorf("failed to insert flow job status: %w", err)
		}
	} else {
		_, err := syncRecordsTx.Exec(c.ctx,
			fmt.Sprintf(updateMetadataForSyncRecordsSQL, internalSchema, mirrorJobsTableIdentifier),
			lastCP, syncBatchID, flowJobName)
		if err != nil {
			return fmt.Errorf("failed to update flow job status: %w", err)
		}
	}

	return nil
}

func (c *PostgresConnector) updateNormalizeMetadata(flowJobName string, normalizeBatchID int64,
	normalizeRecordsTx pgx.Tx) error {
	jobMetadataExists, err := c.jobMetadataExists(flowJobName)
	if err != nil {
		return fmt.Errorf("failed to get sync status for flow job: %w", err)
	}
	if !jobMetadataExists {
		return fmt.Errorf("job metadata does not exist, unable to update")
	}

	_, err = normalizeRecordsTx.Exec(c.ctx,
		fmt.Sprintf(updateMetadataForNormalizeRecordsSQL, internalSchema, mirrorJobsTableIdentifier),
		normalizeBatchID, flowJobName)
	if err != nil {
		return fmt.Errorf("failed to update metadata for NormalizeTables: %w", err)
	}

	return nil
}

func (c *PostgresConnector) getTableNametoUnchangedCols(flowJobName string, syncBatchID int64,
	normalizeBatchID int64) (map[string][]string, error) {
	rawTableIdentifier := getRawTableIdentifier(flowJobName)

	rows, err := c.pool.Query(c.ctx, fmt.Sprintf(getTableNameToUnchangedToastColsSQL, internalSchema,
		rawTableIdentifier), normalizeBatchID, syncBatchID)
	if err != nil {
		return nil, fmt.Errorf("error while retrieving table names for normalization: %w", err)
	}
	defer rows.Close()

	// Create a map to store the results
	resultMap := make(map[string][]string)
	var destinationTableName string
	var unchangedToastColumns []string
	// Process the rows and populate the map
	for rows.Next() {
		err := rows.Scan(&destinationTableName, &unchangedToastColumns)
		if err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}
		resultMap[destinationTableName] = unchangedToastColumns
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating over rows: %v", err)
	}
	return resultMap, nil
}

func (c *PostgresConnector) generateMergeStatement(destinationTableIdentifier string, unchangedToastColumns []string,
	rawTableIdentifier string) string {
	normalizedTableSchema := c.tableSchemaMapping[destinationTableIdentifier]
	// TODO: switch this to function maps.Keys when it is moved into Go's stdlib
	columnNames := make([]string, 0, len(normalizedTableSchema.Columns))
	for columnName := range normalizedTableSchema.Columns {
		columnNames = append(columnNames, columnName)
	}

	flattenedCastsSQLArray := make([]string, 0, len(normalizedTableSchema.Columns))
	var primaryKeyColumnCast string
	for columnName, genericColumnType := range normalizedTableSchema.Columns {
		pgType := qValueKindToPostgresType(genericColumnType)
		flattenedCastsSQLArray = append(flattenedCastsSQLArray, fmt.Sprintf("(_peerdb_data->>'%s')::%s AS %s",
			columnName, pgType, columnName))
		if normalizedTableSchema.PrimaryKeyColumn == columnName {
			primaryKeyColumnCast = fmt.Sprintf("(_peerdb_data->>'%s')::%s", columnName, pgType)
		}
	}
	flattenedCastsSQL := strings.TrimSuffix(strings.Join(flattenedCastsSQLArray, ","), ",")

	insertColumnsSQL := strings.TrimSuffix(strings.Join(columnNames, ","), ",")
	insertValuesSQLArray := make([]string, 0, len(columnNames))
	for _, columnName := range columnNames {
		insertValuesSQLArray = append(insertValuesSQLArray, fmt.Sprintf("src.%s", columnName))
	}
	insertValuesSQL := strings.TrimSuffix(strings.Join(insertValuesSQLArray, ","), ",")
	updateStatements := c.generateUpdateStatement(columnNames, unchangedToastColumns)

	return fmt.Sprintf(mergeStatementSQL, primaryKeyColumnCast, internalSchema, rawTableIdentifier,
		destinationTableIdentifier, flattenedCastsSQL, normalizedTableSchema.PrimaryKeyColumn,
		normalizedTableSchema.PrimaryKeyColumn, insertColumnsSQL, insertValuesSQL, updateStatements)
}

func (c *PostgresConnector) generateUpdateStatement(allCols []string, unchangedToastColsLists []string) string {
	updateStmts := make([]string, 0)

	for _, cols := range unchangedToastColsLists {
		unchangedColsArray := strings.Split(cols, ",")
		otherCols := utils.ArrayMinus(allCols, unchangedColsArray)
		tmpArray := make([]string, 0)
		for _, colName := range otherCols {
			tmpArray = append(tmpArray, fmt.Sprintf("%s=src.%s", colName, colName))
		}
		ssep := strings.Join(tmpArray, ",")
		updateStmt := fmt.Sprintf(`WHEN MATCHED AND
		src._peerdb_record_type=1 AND _peerdb_unchanged_toast_columns='%s'
		THEN UPDATE SET %s `, cols, ssep)
		updateStmts = append(updateStmts, updateStmt)
	}
	return strings.Join(updateStmts, "\n")
}
