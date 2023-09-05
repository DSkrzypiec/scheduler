package db

import (
	"database/sql"
	"errors"
	"go_shed/src/dag"
	"go_shed/src/version"
	"reflect"
	"time"

	"github.com/rs/zerolog/log"
)

// DagTask represents single row in dagtasks table in the database.
type DagTask struct {
	DagId          string
	TaskId         string
	IsCurrent      bool
	InsertTs       string
	Version        string
	TaskTypeName   string
	TaskBodyHash   string
	TaskBodySource string
}

// InsertDagTasks inserts the tasks of given DAG to dagtasks table and set it as the current version. Previous versions
// would still be in dagtasks table but with set IsCurrent=0. In case of inserting any of dag's task insertion would be
// rollbacked (in terms of SQL transactions).
func (c *Client) InsertDagTasks(d dag.Dag) error {
	start := time.Now()
	dagId := string(d.Attr.Id)
	log.Info().Str("dagId", dagId).Msgf("[%s] Start syncing dag and dagtasks table...", LOG_PREFIX)
	tx, _ := c.dbConn.Begin()

	for _, task := range d.Flatten() {
		iErr := c.insertSingleDagTask(tx, dagId, task)
		if iErr != nil {
			rollErr := tx.Rollback()
			if rollErr != nil {
				log.Error().Err(rollErr).Msgf("[%s] Error while rollbacking SQL transaction", LOG_PREFIX)
			}
			return errors.New("could not sync dag and dagtasks properly. SQL transaction was rollbacked")
		}
	}

	cErr := tx.Commit()
	if cErr != nil {
		log.Error().Err(cErr).Str("dagId", dagId).Msgf("[%s] Could not commit SQL transaction", LOG_PREFIX)
		return cErr
	}

	log.Info().Str("dagId", dagId).Dur("durationMs", time.Since(start)).
		Msgf("[%s] Finished syncing dag and dagtasks table.", LOG_PREFIX)
	return nil
}

// insertSingleDagTask inserts new DagTask which represents a task within a DAG. Using multiple InsertDagTask for dagId and
// task is a common case. Newely inserted version will have IsCurrent=1 and others will not. On database side (DagId,
// TaskId, IsCurrent) defines primary key on dagtasks table.
func (c *Client) insertSingleDagTask(tx *sql.Tx, dagId string, task dag.Task) error {
	start := time.Now()
	insertTs := time.Now().Format(InsertTsFormat)
	log.Info().Str("dagId", dagId).Str("taskId", task.Id()).Str("insertTs", insertTs).Msgf("[%s] Start inserting new DagTask.", LOG_PREFIX)

	// Make IsCurrent=0 for outdated rows
	uErr := c.outdateDagTasks(tx, dagId, task.Id(), insertTs)
	if uErr != nil {
		log.Error().Err(uErr).Str("dagId", dagId).Str("taskId", task.Id()).Msgf("[%s] Cannot outdate old dagtasks", LOG_PREFIX)
		return uErr
	}

	// Insert dagtask row
	iErr := c.insertDagTask(tx, dagId, task, insertTs)
	if iErr != nil {
		log.Error().Err(iErr).Str("dagId", dagId).Str("taskId", task.Id()).Msgf("[%s] Cannot insert new dagtask", LOG_PREFIX)
		return iErr
	}

	log.Info().Str("dagId", dagId).Str("taskId", task.Id()).Dur("durationMs", time.Since(start)).
		Msgf("[%s] Finished inserting new DagTask.", LOG_PREFIX)
	return nil
}

// Insert new row in dagtasks table.
func (c *Client) insertDagTask(tx *sql.Tx, dagId string, task dag.Task, insertTs string) error {
	tTypeName := reflect.TypeOf(task).Name()
	taskBody := dag.TaskExecuteSource(task)
	taskHash := dag.TaskHash(task)

	_, err := tx.Exec(
		c.dagTaskInsertQuery(),
		dagId, task.Id(), 1, insertTs, version.Version, tTypeName, taskHash, taskBody,
	)
	if err != nil {
		return err
	}
	return nil
}

// Outdates all row in dagtasks table for given dagId and taskId which are older then given currentInsertTs timestamp.
func (c *Client) outdateDagTasks(tx *sql.Tx, dagId string, taskId string, currentInsertTs string) error {
	_, err := tx.Exec(c.dagTaskOutdateQuery(), dagId, taskId, currentInsertTs)
	if err != nil {
		return err
	}
	return nil
}

// ReadDagTask reads single row (current version) from dagtasks table for given DAG ID and task ID.
func (c *Client) ReadDagTask(dagId, taskId string) (DagTask, error) {
	start := time.Now()
	log.Info().Str("dagId", dagId).Str("taskId", taskId).Msgf("[%s] Start reading DagTask.", LOG_PREFIX)
	row := c.dbConn.QueryRow(c.readDagTaskQuery(), dagId, taskId)

	var dId, tId, typeName, insertTs, version, bodyHash, bodySource string
	var isCurrent int
	scanErr := row.Scan(&dId, &tId, &isCurrent, &insertTs, &version, &typeName, &bodyHash, &bodySource)

	if scanErr == sql.ErrNoRows {
		return DagTask{}, scanErr
	}

	if scanErr != nil {
		log.Error().Err(scanErr).Str("dagId", dagId).Str("taskId", taskId).
			Msgf("[%s] failed scanning dagtask record", LOG_PREFIX)
		return DagTask{}, scanErr
	}

	dagtask := DagTask{
		DagId:          dId,
		TaskId:         tId,
		IsCurrent:      isCurrent == 1,
		Version:        version,
		TaskTypeName:   typeName,
		TaskBodyHash:   bodyHash,
		TaskBodySource: bodySource,
	}

	log.Info().Str("dagId", dagId).Str("taskId", taskId).Dur("durationMs", time.Since(start)).
		Msgf("[%s] Finished reading DagTask.", LOG_PREFIX)
	return dagtask, nil
}

func (c *Client) readDagTaskQuery() string {
	return `
		SELECT
			DagId,
			TaskId,
			IsCurrent,
			InsertTs,
			Version,
			TaskTypeName,
			TaskBodyHash,
			TaskBodySource
		FROM
			dagtasks
		WHERE
				dagId = ?
			AND taskId = ?
			AND IsCurrent = 1
	`
}

func (c *Client) dagTaskInsertQuery() string {
	return `
		INSERT INTO dagtasks (
			DagId, TaskId, IsCurrent, InsertTs, Version, TaskTypeName, TaskBodyHash, TaskBodySource
		)
		VALUES (?,?,?,?,?,?,?,?)
	`
}

func (c *Client) dagTaskOutdateQuery() string {
	return `
		UPDATE
			dagtasks
		SET
			IsCurrent = 0
		WHERE
				DagId = ?
			AND TaskId = ?
			AND InsertTs != ?
	`
}
