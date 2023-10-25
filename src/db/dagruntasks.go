package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/rs/zerolog/log"
)

type DagRunTask struct {
	DagId          string
	ExecTs         string
	TaskId         string
	InsertTs       string
	Status         string
	StatusUpdateTs string
	Version        string
}

// Reads DAG run tasks information from dagruntasks table for given DAG run.
func (c *Client) ReadDagRunTasks(ctx context.Context, dagId, execTs string) ([]DagRunTask, error) {
	start := time.Now()
	log.Info().Str("dagId", dagId).Str("execTs", execTs).
		Msgf("[%s] Start reading DAG run tasks...", LOG_PREFIX)
	dagruntasks := make([]DagRunTask, 0, 100)

	rows, qErr := c.dbConn.QueryContext(ctx, c.readDagRunTasksQuery(), dagId, execTs)
	if qErr != nil {
		log.Error().Err(qErr).Str("dagId", dagId).Str("execTs", execTs).
			Msgf("[%s] Failed querying DAG runs.", LOG_PREFIX)
		return nil, qErr
	}
	defer rows.Close()

	for rows.Next() {
		select {
		case <-ctx.Done():
			// Handle context cancellation or deadline exceeded
			log.Warn().Err(ctx.Err()).Str("dagId", dagId).Str("execTs", execTs).
				Msgf("[%s] Context done while processing rows.", LOG_PREFIX)
			return nil, ctx.Err()
		default:
		}

		dagruntask, scanErr := parseDagRunTask(rows)
		if scanErr != nil {
			log.Error().Err(scanErr).Str("dagId", dagId).Str("execTs", execTs).
				Msgf("[%s] Failed scanning a DagRunTask record.", LOG_PREFIX)
			return nil, scanErr
		}
		dagruntasks = append(dagruntasks, dagruntask)
	}

	log.Info().Str("dagId", dagId).Str("execTs", execTs).
		Dur("durationMs", time.Since(start)).
		Msgf("[%s] Finished reading DAG runs.", LOG_PREFIX)
	return dagruntasks, nil
}

// Inserts new DagRunTask with default status SCHEDULED.
func (c *Client) InsertDagRunTask(ctx context.Context, dagId, execTs, taskId string) error {
	// TODO
	return nil
}

func parseDagRunTask(rows *sql.Rows) (DagRunTask, error) {
	var dagId, execTs, taskId, insertTs, status, statusTs, version string
	scanErr := rows.Scan(&dagId, &execTs, &taskId, &insertTs, &status,
		&statusTs, &version)
	if scanErr != nil {
		return DagRunTask{}, scanErr
	}
	dagRunTask := DagRunTask{
		DagId:          dagId,
		ExecTs:         execTs,
		TaskId:         taskId,
		InsertTs:       insertTs,
		Status:         status,
		StatusUpdateTs: statusTs,
		Version:        version,
	}
	return dagRunTask, nil
}

func (c *Client) readDagRunTasksQuery() string {
	return `
	SELECT
		DagId,
		ExecTs,
		TaskId,
		InsertTs,
		Status,
		StatusUpdateTs,
		Version
	FROM
		dagruntasks
	WHERE
			DagId = ?
		AND ExecTs = ?
	`
}