package db

import (
	"database/sql"
	"encoding/json"
	"go_shed/src/dag"
	"go_shed/src/timeutils"
	"go_shed/src/version"
	"time"

	"github.com/rs/zerolog/log"
)

type Dag struct {
	DagId               string
	StartTs             *string
	Schedule            *string
	CreateTs            string
	LatestUpdateTs      *string
	CreateVersion       string
	LatestUpdateVersion *string
	HashDagMeta         string
	HashTasks           string
	Attributes          string // serialized dag.Dag.Attr
}

// ReadDag reads metadata about DAG from dags table for given dagId.
func (c *Client) ReadDag(dagId string) (Dag, error) {
	tx, _ := c.dbConn.Begin()
	d, err := c.readDagTx(tx, dagId)
	cErr := tx.Commit()
	if cErr != nil {
		log.Error().Err(cErr).Str("dagId", dagId).Msgf("[%s] Could not commit SQL transaction", LOG_PREFIX)
		return Dag{}, cErr
	}
	return d, err
}

// Upsert inserts or updates DAG details in dags table.
func (c *Client) UpsertDag(d dag.Dag) error {
	start := time.Now()
	insertTs := timeutils.ToString(time.Now())
	dagId := string(d.Id)
	log.Info().Str("dagId", dagId).Str("insertTs", insertTs).Msgf("[%s] Start upserting dag...", LOG_PREFIX)
	tx, _ := c.dbConn.Begin()

	// Check if there is already a record for given DAG
	currDagRow, currErr := c.readDagTx(tx, dagId)
	if currErr == sql.ErrNoRows {
		// If no, then simply insert
		dag := fromDagToDag(d, insertTs)
		iErr := c.insertDag(tx, dag, insertTs)
		cErr := tx.Commit()
		if cErr != nil {
			log.Info().Str("dagId", dagId).Dur("durationMs", time.Since(start)).Err(cErr).Msgf("[%s] Could not commit SQL transaction", LOG_PREFIX)
			tx.Rollback()
			return cErr
		}
		log.Info().Str("dagId", dagId).Dur("durationMs", time.Since(start)).Msgf("[%s] Inserted new DAG into dags table", LOG_PREFIX)
		return iErr
	}
	// Otherwise we need to update existing entry in dags table
	updatedDag := dagUpdate(d, currDagRow, insertTs)
	uErr := c.updateDag(tx, updatedDag)

	cErr := tx.Commit()
	if cErr != nil {
		log.Info().Str("dagId", dagId).Dur("durationMs", time.Since(start)).Err(cErr).Msgf("[%s] Could not commit SQL transaction", LOG_PREFIX)
		tx.Rollback()
		return cErr
	}
	log.Info().Str("dagId", dagId).Dur("durationMs", time.Since(start)).Msgf("[%s] Updateing DAG row in dags table", LOG_PREFIX)
	return uErr
}

// readDag reads a row from dags table within SQL transaction.
func (c *Client) readDagTx(tx *sql.Tx, dagId string) (Dag, error) {
	start := time.Now()
	log.Info().Str("dagId", dagId).Msgf("[%s] Start reading Dag.", LOG_PREFIX)

	row := tx.QueryRow(c.readDagQuery(), dagId)
	var dId, createTs, createVersion, hashMeta, hashTasks, attr string
	var startTs, schedule, latestUpdateTs, latestUpdateVersion *string

	scanErr := row.Scan(&dId, &startTs, &schedule, &createTs, &latestUpdateTs, &createVersion, &latestUpdateVersion,
		&hashMeta, &hashTasks, &attr)
	if scanErr == sql.ErrNoRows {
		return Dag{}, scanErr
	}
	if scanErr != nil {
		log.Error().Err(scanErr).Str("dagId", dagId).Msgf("[%s] failed scanning dag record", LOG_PREFIX)
		return Dag{}, scanErr
	}
	dag := Dag{
		DagId:               dId,
		StartTs:             startTs,
		Schedule:            schedule,
		CreateTs:            createTs,
		LatestUpdateTs:      latestUpdateTs,
		CreateVersion:       createVersion,
		LatestUpdateVersion: latestUpdateVersion,
		HashDagMeta:         hashMeta,
		HashTasks:           hashTasks,
		Attributes:          attr,
	}
	log.Info().Dur("durationMs", time.Since(start)).Msgf("[%s] Finished reading Dag.", LOG_PREFIX)
	return dag, nil
}

// Insert new row in dags table.
func (c *Client) insertDag(tx *sql.Tx, d Dag, insertTs string) error {
	_, err := tx.Exec(
		c.dagInsertQuery(),
		d.DagId, d.StartTs, d.Schedule, d.CreateTs, d.LatestUpdateTs, d.CreateVersion, d.LatestUpdateVersion, d.HashDagMeta,
		d.HashTasks, d.Attributes,
	)
	if err != nil {
		return err
	}
	return nil
}

// Updates existing row in dags table.
func (c *Client) updateDag(tx *sql.Tx, d Dag) error {
	_, err := tx.Exec(
		c.dagUpdateQuery(),
		d.StartTs, d.Schedule, d.LatestUpdateTs, d.LatestUpdateVersion, d.HashDagMeta, d.HashTasks, d.Attributes, d.DagId,
	)
	if err != nil {
		return err
	}
	return nil
}

func fromDagToDag(d dag.Dag, createTs string) Dag {
	attrJson, jErr := json.Marshal(d.Attr)
	if jErr != nil {
		attrJson = []byte("FAILED DAG ATTR SERIALIZATION")
	}
	var dagStart, sched *string
	if d.Schedule != nil {
		schedStr := (*d.Schedule).String()
		sched = &schedStr
		startStr := timeutils.ToString((*d.Schedule).StartTime())
		dagStart = &startStr
	}
	return Dag{
		DagId:               string(d.Id),
		StartTs:             dagStart,
		Schedule:            sched,
		CreateTs:            createTs,
		LatestUpdateTs:      nil,
		CreateVersion:       version.Version,
		LatestUpdateVersion: nil,
		HashDagMeta:         d.HashDagMeta(),
		HashTasks:           d.HashTasks(),
		Attributes:          string(attrJson),
	}
}

func dagUpdate(d dag.Dag, currDagRow Dag, insertTs string) Dag {
	attrJson, jErr := json.Marshal(d.Attr)
	if jErr != nil {
		attrJson = []byte("FAILED DAG ATTR SERIALIZATION")
	}
	return Dag{
		DagId:               string(d.Id),
		StartTs:             currDagRow.StartTs,
		Schedule:            currDagRow.Schedule,
		CreateTs:            currDagRow.CreateTs,
		LatestUpdateTs:      &insertTs,
		CreateVersion:       currDagRow.CreateVersion,
		LatestUpdateVersion: &version.Version,
		HashDagMeta:         d.HashDagMeta(),
		HashTasks:           d.HashTasks(),
		Attributes:          string(attrJson),
	}
}

func (c *Client) readDagQuery() string {
	return `
		SELECT
			DagId,
			StartTs,
			Schedule,
			CreateTs,
			LatestUpdateTs,
			CreateVersion,
			LatestUpdateVersion,
			HashDagMeta,
			HashTasks,
			Attributes
		FROM
			dags
		WHERE
			DagId = ?
	`
}

func (c *Client) dagInsertQuery() string {
	return `
		INSERT INTO dags (
			DagId, StartTs, Schedule, CreateTs, LatestUpdateTs, CreateVersion, LatestUpdateVersion, HashDagMeta, HashTasks, Attributes
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
}

func (c *Client) dagUpdateQuery() string {
	return `
		UPDATE
			dags
		SET
			StartTs = ?,
			Schedule = ?,
			LatestUpdateTs = ?,
			LatestUpdateVersion = ?,
			HashDagMeta = ?,
			HashTasks = ?,
			Attributes = ?
		WHERE
			DagId = ?
	`
}

// TODO: Move somewhere?
func pointerEqual[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func (d Dag) Equals(e Dag) bool {
	if d.DagId != e.DagId {
		return false
	}
	if !pointerEqual(d.StartTs, e.StartTs) {
		return false
	}
	if !pointerEqual(d.Schedule, e.Schedule) {
		return false
	}
	if d.CreateTs != e.CreateTs {
		return false
	}
	if !pointerEqual(d.LatestUpdateTs, e.LatestUpdateTs) {
		return false
	}
	if d.CreateVersion != e.CreateVersion {
		return false
	}
	if !pointerEqual(d.LatestUpdateVersion, e.LatestUpdateVersion) {
		return false
	}
	if d.HashDagMeta != e.HashDagMeta {
		return false
	}
	if d.HashTasks != e.HashTasks {
		return false
	}
	if d.Attributes != e.Attributes {
		return false
	}
	return true
}
