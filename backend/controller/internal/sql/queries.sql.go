// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.21.0
// source: queries.sql

package sql

import (
	"context"
	"encoding/json"
	"time"

	"github.com/TBD54566975/ftl/backend/common/model"
	"github.com/TBD54566975/ftl/backend/controller/internal/sqltypes"
	"github.com/alecthomas/types"
)

const associateArtefactWithDeployment = `-- name: AssociateArtefactWithDeployment :exec
INSERT INTO deployment_artefacts (deployment_id, artefact_id, executable, path)
VALUES ((SELECT id FROM deployments WHERE name = $1), $2, $3, $4)
`

type AssociateArtefactWithDeploymentParams struct {
	Name       model.DeploymentName
	ArtefactID int64
	Executable bool
	Path       string
}

func (q *Queries) AssociateArtefactWithDeployment(ctx context.Context, arg AssociateArtefactWithDeploymentParams) error {
	_, err := q.db.Exec(ctx, associateArtefactWithDeployment,
		arg.Name,
		arg.ArtefactID,
		arg.Executable,
		arg.Path,
	)
	return err
}

const createArtefact = `-- name: CreateArtefact :one
INSERT INTO artefacts (digest, content)
VALUES ($1, $2)
RETURNING id
`

// Create a new artefact and return the artefact ID.
func (q *Queries) CreateArtefact(ctx context.Context, digest []byte, content []byte) (int64, error) {
	row := q.db.QueryRow(ctx, createArtefact, digest, content)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const createDeployment = `-- name: CreateDeployment :exec
INSERT INTO deployments (module_id, "schema", name)
VALUES ((SELECT id FROM modules WHERE name = $2::TEXT LIMIT 1), $3::BYTEA, $1)
`

func (q *Queries) CreateDeployment(ctx context.Context, name model.DeploymentName, moduleName string, schema []byte) error {
	_, err := q.db.Exec(ctx, createDeployment, name, moduleName, schema)
	return err
}

const createIngressRequest = `-- name: CreateIngressRequest :exec
INSERT INTO requests (origin, name, source_addr)
VALUES ($1, $2, $3)
`

func (q *Queries) CreateIngressRequest(ctx context.Context, origin Origin, name string, sourceAddr string) error {
	_, err := q.db.Exec(ctx, createIngressRequest, origin, name, sourceAddr)
	return err
}

const createIngressRoute = `-- name: CreateIngressRoute :exec
INSERT INTO ingress_routes (deployment_id, module, verb, method, path)
VALUES ((SELECT id FROM deployments WHERE name = $1 LIMIT 1), $2, $3, $4, $5)
`

type CreateIngressRouteParams struct {
	Name   model.DeploymentName
	Module string
	Verb   string
	Method string
	Path   string
}

func (q *Queries) CreateIngressRoute(ctx context.Context, arg CreateIngressRouteParams) error {
	_, err := q.db.Exec(ctx, createIngressRoute,
		arg.Name,
		arg.Module,
		arg.Verb,
		arg.Method,
		arg.Path,
	)
	return err
}

const deregisterRunner = `-- name: DeregisterRunner :one
WITH matches AS (
    UPDATE runners
        SET state = 'dead'
        WHERE key = $1
        RETURNING 1)
SELECT COUNT(*)
FROM matches
`

func (q *Queries) DeregisterRunner(ctx context.Context, key sqltypes.Key) (int64, error) {
	row := q.db.QueryRow(ctx, deregisterRunner, key)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const expireRunnerReservations = `-- name: ExpireRunnerReservations :one
WITH rows AS (
    UPDATE runners
        SET state = 'idle',
            deployment_id = NULL,
            reservation_timeout = NULL
        WHERE state = 'reserved'
            AND reservation_timeout < (NOW() AT TIME ZONE 'utc')
        RETURNING 1)
SELECT COUNT(*)
FROM rows
`

func (q *Queries) ExpireRunnerReservations(ctx context.Context) (int64, error) {
	row := q.db.QueryRow(ctx, expireRunnerReservations)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const getActiveRunners = `-- name: GetActiveRunners :many
SELECT DISTINCT ON (r.key) r.key                                   AS runner_key,
                           r.endpoint,
                           r.state,
                           r.labels,
                           r.last_seen,
                           r.module_name,
                           COALESCE(CASE
                                        WHEN r.deployment_id IS NOT NULL
                                            THEN d.name END, NULL) AS deployment_name
FROM runners r
         LEFT JOIN deployments d on d.id = r.deployment_id
WHERE $1::bool = true
   OR r.state <> 'dead'
ORDER BY r.key
`

type GetActiveRunnersRow struct {
	RunnerKey      sqltypes.Key
	Endpoint       string
	State          RunnerState
	Labels         []byte
	LastSeen       time.Time
	ModuleName     types.Option[string]
	DeploymentName interface{}
}

func (q *Queries) GetActiveRunners(ctx context.Context, all bool) ([]GetActiveRunnersRow, error) {
	rows, err := q.db.Query(ctx, getActiveRunners, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetActiveRunnersRow
	for rows.Next() {
		var i GetActiveRunnersRow
		if err := rows.Scan(
			&i.RunnerKey,
			&i.Endpoint,
			&i.State,
			&i.Labels,
			&i.LastSeen,
			&i.ModuleName,
			&i.DeploymentName,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getAllIngressRoutes = `-- name: GetAllIngressRoutes :many
SELECT d.name AS deployment_name, ir.module, ir.verb, ir.method, ir.path
FROM ingress_routes ir
         INNER JOIN deployments d ON ir.deployment_id = d.id
WHERE $1::bool = true
   OR d.min_replicas > 0
`

type GetAllIngressRoutesRow struct {
	DeploymentName model.DeploymentName
	Module         string
	Verb           string
	Method         string
	Path           string
}

func (q *Queries) GetAllIngressRoutes(ctx context.Context, all bool) ([]GetAllIngressRoutesRow, error) {
	rows, err := q.db.Query(ctx, getAllIngressRoutes, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetAllIngressRoutesRow
	for rows.Next() {
		var i GetAllIngressRoutesRow
		if err := rows.Scan(
			&i.DeploymentName,
			&i.Module,
			&i.Verb,
			&i.Method,
			&i.Path,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getArtefactContentRange = `-- name: GetArtefactContentRange :one
SELECT SUBSTRING(a.content FROM $1 FOR $2)::BYTEA AS content
FROM artefacts a
WHERE a.id = $3
`

func (q *Queries) GetArtefactContentRange(ctx context.Context, start int32, count int32, iD int64) ([]byte, error) {
	row := q.db.QueryRow(ctx, getArtefactContentRange, start, count, iD)
	var content []byte
	err := row.Scan(&content)
	return content, err
}

const getArtefactDigests = `-- name: GetArtefactDigests :many
SELECT id, digest
FROM artefacts
WHERE digest = ANY ($1::bytea[])
`

type GetArtefactDigestsRow struct {
	ID     int64
	Digest []byte
}

// Return the digests that exist in the database.
func (q *Queries) GetArtefactDigests(ctx context.Context, digests [][]byte) ([]GetArtefactDigestsRow, error) {
	rows, err := q.db.Query(ctx, getArtefactDigests, digests)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetArtefactDigestsRow
	for rows.Next() {
		var i GetArtefactDigestsRow
		if err := rows.Scan(&i.ID, &i.Digest); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getControllers = `-- name: GetControllers :many
SELECT id, key, created, last_seen, state, endpoint
FROM controller c
WHERE $1::bool = true
   OR c.state <> 'dead'
ORDER BY c.key
`

func (q *Queries) GetControllers(ctx context.Context, all bool) ([]Controller, error) {
	rows, err := q.db.Query(ctx, getControllers, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Controller
	for rows.Next() {
		var i Controller
		if err := rows.Scan(
			&i.ID,
			&i.Key,
			&i.Created,
			&i.LastSeen,
			&i.State,
			&i.Endpoint,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getDeployment = `-- name: GetDeployment :one
SELECT d.id, d.created_at, d.module_id, d.name, d.schema, d.labels, d.min_replicas, m.language, m.name AS module_name, d.min_replicas
FROM deployments d
         INNER JOIN modules m ON m.id = d.module_id
WHERE d.name = $1
`

type GetDeploymentRow struct {
	Deployment  Deployment
	Language    string
	ModuleName  string
	MinReplicas int32
}

func (q *Queries) GetDeployment(ctx context.Context, name model.DeploymentName) (GetDeploymentRow, error) {
	row := q.db.QueryRow(ctx, getDeployment, name)
	var i GetDeploymentRow
	err := row.Scan(
		&i.Deployment.ID,
		&i.Deployment.CreatedAt,
		&i.Deployment.ModuleID,
		&i.Deployment.Name,
		&i.Deployment.Schema,
		&i.Deployment.Labels,
		&i.Deployment.MinReplicas,
		&i.Language,
		&i.ModuleName,
		&i.MinReplicas,
	)
	return i, err
}

const getDeploymentArtefacts = `-- name: GetDeploymentArtefacts :many
SELECT da.created_at, artefact_id AS id, executable, path, digest, executable
FROM deployment_artefacts da
         INNER JOIN artefacts ON artefacts.id = da.artefact_id
WHERE deployment_id = $1
`

type GetDeploymentArtefactsRow struct {
	CreatedAt    time.Time
	ID           int64
	Executable   bool
	Path         string
	Digest       []byte
	Executable_2 bool
}

// Get all artefacts matching the given digests.
func (q *Queries) GetDeploymentArtefacts(ctx context.Context, deploymentID int64) ([]GetDeploymentArtefactsRow, error) {
	rows, err := q.db.Query(ctx, getDeploymentArtefacts, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetDeploymentArtefactsRow
	for rows.Next() {
		var i GetDeploymentArtefactsRow
		if err := rows.Scan(
			&i.CreatedAt,
			&i.ID,
			&i.Executable,
			&i.Path,
			&i.Digest,
			&i.Executable_2,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getDeployments = `-- name: GetDeployments :many
SELECT d.id, d.created_at, d.module_id, d.name, d.schema, d.labels, d.min_replicas, m.name AS module_name, m.language
FROM deployments d
         INNER JOIN modules m on d.module_id = m.id
WHERE $1::bool = true
   OR min_replicas > 0
ORDER BY d.name
`

type GetDeploymentsRow struct {
	Deployment Deployment
	ModuleName string
	Language   string
}

func (q *Queries) GetDeployments(ctx context.Context, all bool) ([]GetDeploymentsRow, error) {
	rows, err := q.db.Query(ctx, getDeployments, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetDeploymentsRow
	for rows.Next() {
		var i GetDeploymentsRow
		if err := rows.Scan(
			&i.Deployment.ID,
			&i.Deployment.CreatedAt,
			&i.Deployment.ModuleID,
			&i.Deployment.Name,
			&i.Deployment.Schema,
			&i.Deployment.Labels,
			&i.Deployment.MinReplicas,
			&i.ModuleName,
			&i.Language,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getDeploymentsByID = `-- name: GetDeploymentsByID :many
SELECT id, created_at, module_id, name, schema, labels, min_replicas
FROM deployments
WHERE id = ANY ($1::BIGINT[])
`

func (q *Queries) GetDeploymentsByID(ctx context.Context, ids []int64) ([]Deployment, error) {
	rows, err := q.db.Query(ctx, getDeploymentsByID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Deployment
	for rows.Next() {
		var i Deployment
		if err := rows.Scan(
			&i.ID,
			&i.CreatedAt,
			&i.ModuleID,
			&i.Name,
			&i.Schema,
			&i.Labels,
			&i.MinReplicas,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getDeploymentsNeedingReconciliation = `-- name: GetDeploymentsNeedingReconciliation :many
SELECT d.name                 AS deployment_name,
       m.name                 AS module_name,
       m.language             AS language,
       COUNT(r.id)            AS assigned_runners_count,
       d.min_replicas::BIGINT AS required_runners_count
FROM deployments d
         LEFT JOIN runners r ON d.id = r.deployment_id AND r.state <> 'dead'
         JOIN modules m ON d.module_id = m.id
GROUP BY d.name, d.min_replicas, m.name, m.language
HAVING COUNT(r.id) <> d.min_replicas
`

type GetDeploymentsNeedingReconciliationRow struct {
	DeploymentName       model.DeploymentName
	ModuleName           string
	Language             string
	AssignedRunnersCount int64
	RequiredRunnersCount int64
}

// Get deployments that have a mismatch between the number of assigned and required replicas.
func (q *Queries) GetDeploymentsNeedingReconciliation(ctx context.Context) ([]GetDeploymentsNeedingReconciliationRow, error) {
	rows, err := q.db.Query(ctx, getDeploymentsNeedingReconciliation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetDeploymentsNeedingReconciliationRow
	for rows.Next() {
		var i GetDeploymentsNeedingReconciliationRow
		if err := rows.Scan(
			&i.DeploymentName,
			&i.ModuleName,
			&i.Language,
			&i.AssignedRunnersCount,
			&i.RequiredRunnersCount,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getDeploymentsWithArtefacts = `-- name: GetDeploymentsWithArtefacts :many
SELECT d.id, d.created_at, d.name as deployment_name, m.name AS module_name
FROM deployments d
         INNER JOIN modules m ON d.module_id = m.id
WHERE EXISTS (SELECT 1
              FROM deployment_artefacts da
                       INNER JOIN artefacts a ON da.artefact_id = a.id
              WHERE a.digest = ANY ($1::bytea[])
                AND da.deployment_id = d.id
              HAVING COUNT(*) = $2 -- Number of unique digests provided
)
`

type GetDeploymentsWithArtefactsRow struct {
	ID             int64
	CreatedAt      time.Time
	DeploymentName model.DeploymentName
	ModuleName     string
}

// Get all deployments that have artefacts matching the given digests.
func (q *Queries) GetDeploymentsWithArtefacts(ctx context.Context, digests [][]byte, count interface{}) ([]GetDeploymentsWithArtefactsRow, error) {
	rows, err := q.db.Query(ctx, getDeploymentsWithArtefacts, digests, count)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetDeploymentsWithArtefactsRow
	for rows.Next() {
		var i GetDeploymentsWithArtefactsRow
		if err := rows.Scan(
			&i.ID,
			&i.CreatedAt,
			&i.DeploymentName,
			&i.ModuleName,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getExistingDeploymentForModule = `-- name: GetExistingDeploymentForModule :one
SELECT d.id, created_at, module_id, d.name, schema, labels, min_replicas, m.id, language, m.name
FROM deployments d
         INNER JOIN modules m on d.module_id = m.id
WHERE m.name = $1
  AND min_replicas > 0
LIMIT 1
`

type GetExistingDeploymentForModuleRow struct {
	ID          int64
	CreatedAt   time.Time
	ModuleID    int64
	Name        model.DeploymentName
	Schema      []byte
	Labels      []byte
	MinReplicas int32
	ID_2        int64
	Language    string
	Name_2      string
}

func (q *Queries) GetExistingDeploymentForModule(ctx context.Context, name string) (GetExistingDeploymentForModuleRow, error) {
	row := q.db.QueryRow(ctx, getExistingDeploymentForModule, name)
	var i GetExistingDeploymentForModuleRow
	err := row.Scan(
		&i.ID,
		&i.CreatedAt,
		&i.ModuleID,
		&i.Name,
		&i.Schema,
		&i.Labels,
		&i.MinReplicas,
		&i.ID_2,
		&i.Language,
		&i.Name_2,
	)
	return i, err
}

const getIdleRunners = `-- name: GetIdleRunners :many
SELECT id, key, created, last_seen, reservation_timeout, state, endpoint, module_name, deployment_id, labels
FROM runners
WHERE labels @> $1::jsonb
  AND state = 'idle'
LIMIT $2
`

func (q *Queries) GetIdleRunners(ctx context.Context, labels []byte, limit int32) ([]Runner, error) {
	rows, err := q.db.Query(ctx, getIdleRunners, labels, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Runner
	for rows.Next() {
		var i Runner
		if err := rows.Scan(
			&i.ID,
			&i.Key,
			&i.Created,
			&i.LastSeen,
			&i.ReservationTimeout,
			&i.State,
			&i.Endpoint,
			&i.ModuleName,
			&i.DeploymentID,
			&i.Labels,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getIngressRoutes = `-- name: GetIngressRoutes :many
SELECT r.key AS runner_key, endpoint, ir.module, ir.verb
FROM ingress_routes ir
         INNER JOIN runners r ON ir.deployment_id = r.deployment_id
WHERE r.state = 'assigned'
  AND ir.method = $1
  AND ir.path = $2
`

type GetIngressRoutesRow struct {
	RunnerKey sqltypes.Key
	Endpoint  string
	Module    string
	Verb      string
}

// Get the runner endpoints corresponding to the given ingress route.
func (q *Queries) GetIngressRoutes(ctx context.Context, method string, path string) ([]GetIngressRoutesRow, error) {
	rows, err := q.db.Query(ctx, getIngressRoutes, method, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetIngressRoutesRow
	for rows.Next() {
		var i GetIngressRoutesRow
		if err := rows.Scan(
			&i.RunnerKey,
			&i.Endpoint,
			&i.Module,
			&i.Verb,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getModulesByID = `-- name: GetModulesByID :many
SELECT id, language, name
FROM modules
WHERE id = ANY ($1::BIGINT[])
`

func (q *Queries) GetModulesByID(ctx context.Context, ids []int64) ([]Module, error) {
	rows, err := q.db.Query(ctx, getModulesByID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Module
	for rows.Next() {
		var i Module
		if err := rows.Scan(&i.ID, &i.Language, &i.Name); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getProcessList = `-- name: GetProcessList :many
SELECT d.min_replicas,
       d.name   AS deployment_name,
       d.labels    deployment_labels,
       r.key    AS runner_key,
       r.endpoint,
       r.labels AS runner_labels
FROM deployments d
         LEFT JOIN runners r on d.id = r.deployment_id
WHERE d.min_replicas > 0
ORDER BY d.name
`

type GetProcessListRow struct {
	MinReplicas      int32
	DeploymentName   model.DeploymentName
	DeploymentLabels []byte
	RunnerKey        sqltypes.NullKey
	Endpoint         types.Option[string]
	RunnerLabels     []byte
}

func (q *Queries) GetProcessList(ctx context.Context) ([]GetProcessListRow, error) {
	rows, err := q.db.Query(ctx, getProcessList)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetProcessListRow
	for rows.Next() {
		var i GetProcessListRow
		if err := rows.Scan(
			&i.MinReplicas,
			&i.DeploymentName,
			&i.DeploymentLabels,
			&i.RunnerKey,
			&i.Endpoint,
			&i.RunnerLabels,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getRouteForRunner = `-- name: GetRouteForRunner :one
SELECT endpoint, r.key AS runner_key, r.module_name, d.name deployment_name, r.state
FROM runners r
         LEFT JOIN deployments d on r.deployment_id = d.id
WHERE r.key = $1
`

type GetRouteForRunnerRow struct {
	Endpoint       string
	RunnerKey      sqltypes.Key
	ModuleName     types.Option[string]
	DeploymentName model.DeploymentName
	State          RunnerState
}

// Retrieve routing information for a runner.
func (q *Queries) GetRouteForRunner(ctx context.Context, key sqltypes.Key) (GetRouteForRunnerRow, error) {
	row := q.db.QueryRow(ctx, getRouteForRunner, key)
	var i GetRouteForRunnerRow
	err := row.Scan(
		&i.Endpoint,
		&i.RunnerKey,
		&i.ModuleName,
		&i.DeploymentName,
		&i.State,
	)
	return i, err
}

const getRoutingTable = `-- name: GetRoutingTable :many
SELECT endpoint, r.key AS runner_key, r.module_name, d.name deployment_name
FROM runners r
         LEFT JOIN deployments d on r.deployment_id = d.id
WHERE state = 'assigned'
  AND (COALESCE(cardinality($1::TEXT[]), 0) = 0
    OR module_name = ANY ($1::TEXT[]))
`

type GetRoutingTableRow struct {
	Endpoint       string
	RunnerKey      sqltypes.Key
	ModuleName     types.Option[string]
	DeploymentName model.DeploymentName
}

func (q *Queries) GetRoutingTable(ctx context.Context, modules []string) ([]GetRoutingTableRow, error) {
	rows, err := q.db.Query(ctx, getRoutingTable, modules)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetRoutingTableRow
	for rows.Next() {
		var i GetRoutingTableRow
		if err := rows.Scan(
			&i.Endpoint,
			&i.RunnerKey,
			&i.ModuleName,
			&i.DeploymentName,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getRunner = `-- name: GetRunner :one
SELECT DISTINCT ON (r.key) r.key                                   AS runner_key,
                           r.endpoint,
                           r.state,
                           r.labels,
                           r.last_seen,
                           r.module_name,
                           COALESCE(CASE
                                        WHEN r.deployment_id IS NOT NULL
                                            THEN d.name END, NULL) AS deployment_name
FROM runners r
         LEFT JOIN deployments d on d.id = r.deployment_id OR r.deployment_id IS NULL
WHERE r.key = $1
`

type GetRunnerRow struct {
	RunnerKey      sqltypes.Key
	Endpoint       string
	State          RunnerState
	Labels         []byte
	LastSeen       time.Time
	ModuleName     types.Option[string]
	DeploymentName interface{}
}

func (q *Queries) GetRunner(ctx context.Context, key sqltypes.Key) (GetRunnerRow, error) {
	row := q.db.QueryRow(ctx, getRunner, key)
	var i GetRunnerRow
	err := row.Scan(
		&i.RunnerKey,
		&i.Endpoint,
		&i.State,
		&i.Labels,
		&i.LastSeen,
		&i.ModuleName,
		&i.DeploymentName,
	)
	return i, err
}

const getRunnerState = `-- name: GetRunnerState :one
SELECT state
FROM runners
WHERE key = $1
`

func (q *Queries) GetRunnerState(ctx context.Context, key sqltypes.Key) (RunnerState, error) {
	row := q.db.QueryRow(ctx, getRunnerState, key)
	var state RunnerState
	err := row.Scan(&state)
	return state, err
}

const getRunnersForDeployment = `-- name: GetRunnersForDeployment :many
SELECT r.id, key, created, last_seen, reservation_timeout, state, endpoint, module_name, deployment_id, r.labels, d.id, created_at, module_id, name, schema, d.labels, min_replicas
FROM runners r
         INNER JOIN deployments d on r.deployment_id = d.id
WHERE state = 'assigned'
  AND d.name = $1
`

type GetRunnersForDeploymentRow struct {
	ID                 int64
	Key                sqltypes.Key
	Created            time.Time
	LastSeen           time.Time
	ReservationTimeout sqltypes.NullTime
	State              RunnerState
	Endpoint           string
	ModuleName         types.Option[string]
	DeploymentID       types.Option[int64]
	Labels             []byte
	ID_2               int64
	CreatedAt          time.Time
	ModuleID           int64
	Name               model.DeploymentName
	Schema             []byte
	Labels_2           []byte
	MinReplicas        int32
}

func (q *Queries) GetRunnersForDeployment(ctx context.Context, name model.DeploymentName) ([]GetRunnersForDeploymentRow, error) {
	rows, err := q.db.Query(ctx, getRunnersForDeployment, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetRunnersForDeploymentRow
	for rows.Next() {
		var i GetRunnersForDeploymentRow
		if err := rows.Scan(
			&i.ID,
			&i.Key,
			&i.Created,
			&i.LastSeen,
			&i.ReservationTimeout,
			&i.State,
			&i.Endpoint,
			&i.ModuleName,
			&i.DeploymentID,
			&i.Labels,
			&i.ID_2,
			&i.CreatedAt,
			&i.ModuleID,
			&i.Name,
			&i.Schema,
			&i.Labels_2,
			&i.MinReplicas,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const insertCallEvent = `-- name: InsertCallEvent :exec
INSERT INTO events (deployment_id, request_id, time_stamp, type,
                    custom_key_1, custom_key_2, custom_key_3, custom_key_4, payload)
VALUES ((SELECT id FROM deployments WHERE deployments.name = $1::TEXT),
        (CASE
             WHEN $2::TEXT IS NULL THEN NULL
             ELSE (SELECT id FROM requests ir WHERE ir.name = $2::TEXT)
            END),
        $3::TIMESTAMPTZ,
        'call',
        $4::TEXT,
        $5::TEXT,
        $6::TEXT,
        $7::TEXT,
        jsonb_build_object(
                'duration_ms', $8::BIGINT,
                'request', $9::JSONB,
                'response', $10::JSONB,
                'error', $11::TEXT
            ))
`

type InsertCallEventParams struct {
	DeploymentName string
	RequestName    types.Option[string]
	TimeStamp      time.Time
	SourceModule   types.Option[string]
	SourceVerb     types.Option[string]
	DestModule     string
	DestVerb       string
	DurationMs     int64
	Request        []byte
	Response       []byte
	Error          types.Option[string]
}

func (q *Queries) InsertCallEvent(ctx context.Context, arg InsertCallEventParams) error {
	_, err := q.db.Exec(ctx, insertCallEvent,
		arg.DeploymentName,
		arg.RequestName,
		arg.TimeStamp,
		arg.SourceModule,
		arg.SourceVerb,
		arg.DestModule,
		arg.DestVerb,
		arg.DurationMs,
		arg.Request,
		arg.Response,
		arg.Error,
	)
	return err
}

const insertDeploymentCreatedEvent = `-- name: InsertDeploymentCreatedEvent :exec
INSERT INTO events (deployment_id, type, custom_key_1, custom_key_2, payload)
VALUES ((SELECT id
         FROM deployments
         WHERE deployments.name = $1::TEXT),
        'deployment_created',
        $2::TEXT,
        $3::TEXT,
        jsonb_build_object(
                'min_replicas', $4::INT,
                'replaced', $5::TEXT
            ))
`

type InsertDeploymentCreatedEventParams struct {
	DeploymentName string
	Language       string
	ModuleName     string
	MinReplicas    int32
	Replaced       types.Option[string]
}

func (q *Queries) InsertDeploymentCreatedEvent(ctx context.Context, arg InsertDeploymentCreatedEventParams) error {
	_, err := q.db.Exec(ctx, insertDeploymentCreatedEvent,
		arg.DeploymentName,
		arg.Language,
		arg.ModuleName,
		arg.MinReplicas,
		arg.Replaced,
	)
	return err
}

const insertDeploymentUpdatedEvent = `-- name: InsertDeploymentUpdatedEvent :exec
INSERT INTO events (deployment_id, type, custom_key_1, custom_key_2, payload)
VALUES ((SELECT id
         FROM deployments
         WHERE deployments.name = $1::TEXT),
        'deployment_updated',
        $2::TEXT,
        $3::TEXT,
        jsonb_build_object(
                'prev_min_replicas', $4::INT,
                'min_replicas', $5::INT
            ))
`

type InsertDeploymentUpdatedEventParams struct {
	DeploymentName  string
	Language        string
	ModuleName      string
	PrevMinReplicas int32
	MinReplicas     int32
}

func (q *Queries) InsertDeploymentUpdatedEvent(ctx context.Context, arg InsertDeploymentUpdatedEventParams) error {
	_, err := q.db.Exec(ctx, insertDeploymentUpdatedEvent,
		arg.DeploymentName,
		arg.Language,
		arg.ModuleName,
		arg.PrevMinReplicas,
		arg.MinReplicas,
	)
	return err
}

const insertEvent = `-- name: InsertEvent :exec
INSERT INTO events (deployment_id, request_id, type,
                    custom_key_1, custom_key_2, custom_key_3, custom_key_4,
                    payload)
VALUES ($1, $2, $3, $4, $4, $5, $6, $7)
RETURNING id
`

type InsertEventParams struct {
	DeploymentID int64
	RequestID    types.Option[int64]
	Type         EventType
	CustomKey1   types.Option[string]
	CustomKey3   types.Option[string]
	CustomKey4   types.Option[string]
	Payload      json.RawMessage
}

func (q *Queries) InsertEvent(ctx context.Context, arg InsertEventParams) error {
	_, err := q.db.Exec(ctx, insertEvent,
		arg.DeploymentID,
		arg.RequestID,
		arg.Type,
		arg.CustomKey1,
		arg.CustomKey3,
		arg.CustomKey4,
		arg.Payload,
	)
	return err
}

const insertLogEvent = `-- name: InsertLogEvent :exec
INSERT INTO events (deployment_id, request_id, time_stamp, custom_key_1, type, payload)
VALUES ((SELECT id FROM deployments d WHERE d.name = $1 LIMIT 1),
        (CASE
             WHEN $2::TEXT IS NULL THEN NULL
             ELSE (SELECT id FROM requests ir WHERE ir.name = $2::TEXT LIMIT 1)
            END),
        $3::TIMESTAMPTZ,
        $4::INT,
        'log',
        jsonb_build_object(
                'message', $5::TEXT,
                'attributes', $6::JSONB,
                'error', $7::TEXT
            ))
`

type InsertLogEventParams struct {
	DeploymentName model.DeploymentName
	RequestName    types.Option[string]
	TimeStamp      time.Time
	Level          int32
	Message        string
	Attributes     []byte
	Error          types.Option[string]
}

func (q *Queries) InsertLogEvent(ctx context.Context, arg InsertLogEventParams) error {
	_, err := q.db.Exec(ctx, insertLogEvent,
		arg.DeploymentName,
		arg.RequestName,
		arg.TimeStamp,
		arg.Level,
		arg.Message,
		arg.Attributes,
		arg.Error,
	)
	return err
}

const killStaleControllers = `-- name: KillStaleControllers :one
WITH matches AS (
    UPDATE controller
        SET state = 'dead'
        WHERE state <> 'dead' AND last_seen < (NOW() AT TIME ZONE 'utc') - $1::INTERVAL
        RETURNING 1)
SELECT COUNT(*)
FROM matches
`

// Mark any controller entries that haven't been updated recently as dead.
func (q *Queries) KillStaleControllers(ctx context.Context, timeout time.Duration) (int64, error) {
	row := q.db.QueryRow(ctx, killStaleControllers, timeout)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const killStaleRunners = `-- name: KillStaleRunners :one
WITH matches AS (
    UPDATE runners
        SET state = 'dead'
        WHERE state <> 'dead' AND last_seen < (NOW() AT TIME ZONE 'utc') - $1::INTERVAL
        RETURNING 1)
SELECT COUNT(*)
FROM matches
`

func (q *Queries) KillStaleRunners(ctx context.Context, timeout time.Duration) (int64, error) {
	row := q.db.QueryRow(ctx, killStaleRunners, timeout)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const replaceDeployment = `-- name: ReplaceDeployment :one
WITH update_container AS (
    UPDATE deployments AS d
        SET min_replicas = update_deployments.min_replicas
        FROM (VALUES ($1::TEXT, 0),
                     ($2::TEXT, $3::INT))
            AS update_deployments(name, min_replicas)
        WHERE d.name = update_deployments.name
        RETURNING 1)
SELECT COUNT(*)
FROM update_container
`

func (q *Queries) ReplaceDeployment(ctx context.Context, oldDeployment string, newDeployment string, minReplicas int32) (int64, error) {
	row := q.db.QueryRow(ctx, replaceDeployment, oldDeployment, newDeployment, minReplicas)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const reserveRunner = `-- name: ReserveRunner :one
UPDATE runners
SET state               = 'reserved',
    reservation_timeout = $1::timestamptz,
    -- If a deployment is not found, then the deployment ID is -1
    -- and the update will fail due to a FK constraint.
    deployment_id       = COALESCE((SELECT id
                                    FROM deployments d
                                    WHERE d.name = $2
                                    LIMIT 1), -1)
WHERE id = (SELECT id
            FROM runners r
            WHERE r.state = 'idle'
              AND r.labels @> $3::jsonb
            LIMIT 1 FOR UPDATE SKIP LOCKED)
RETURNING runners.id, runners.key, runners.created, runners.last_seen, runners.reservation_timeout, runners.state, runners.endpoint, runners.module_name, runners.deployment_id, runners.labels
`

// Find an idle runner and reserve it for the given deployment.
func (q *Queries) ReserveRunner(ctx context.Context, reservationTimeout time.Time, deploymentName model.DeploymentName, labels []byte) (Runner, error) {
	row := q.db.QueryRow(ctx, reserveRunner, reservationTimeout, deploymentName, labels)
	var i Runner
	err := row.Scan(
		&i.ID,
		&i.Key,
		&i.Created,
		&i.LastSeen,
		&i.ReservationTimeout,
		&i.State,
		&i.Endpoint,
		&i.ModuleName,
		&i.DeploymentID,
		&i.Labels,
	)
	return i, err
}

const setDeploymentDesiredReplicas = `-- name: SetDeploymentDesiredReplicas :exec
UPDATE deployments
SET min_replicas = $2
WHERE name = $1
RETURNING 1
`

func (q *Queries) SetDeploymentDesiredReplicas(ctx context.Context, name model.DeploymentName, minReplicas int32) error {
	_, err := q.db.Exec(ctx, setDeploymentDesiredReplicas, name, minReplicas)
	return err
}

const upsertController = `-- name: UpsertController :one
INSERT INTO controller (key, endpoint)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET state     = 'live',
                                endpoint  = $2,
                                last_seen = NOW() AT TIME ZONE 'utc'
RETURNING id
`

func (q *Queries) UpsertController(ctx context.Context, key model.ControllerKey, endpoint string) (int64, error) {
	row := q.db.QueryRow(ctx, upsertController, key, endpoint)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const upsertModule = `-- name: UpsertModule :one
INSERT INTO modules (language, name)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET language = $1
RETURNING id
`

func (q *Queries) UpsertModule(ctx context.Context, language string, name string) (int64, error) {
	row := q.db.QueryRow(ctx, upsertModule, language, name)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const upsertRunner = `-- name: UpsertRunner :one
WITH deployment_rel AS (
    SELECT CASE
               WHEN $5::TEXT IS NULL
                   THEN NULL
               ELSE COALESCE((SELECT id
                              FROM deployments d
                              WHERE d.name = $5
                              LIMIT 1), -1) END AS id)
INSERT
INTO runners (key, endpoint, state, labels, deployment_id, last_seen)
VALUES ($1,
        $2,
        $3,
        $4,
        (SELECT id FROM deployment_rel),
        NOW() AT TIME ZONE 'utc')
ON CONFLICT (key) DO UPDATE SET endpoint      = $2,
                                state         = $3,
                                labels        = $4,
                                deployment_id = (SELECT id FROM deployment_rel),
                                last_seen     = NOW() AT TIME ZONE 'utc'
RETURNING deployment_id
`

type UpsertRunnerParams struct {
	Key            sqltypes.Key
	Endpoint       string
	State          RunnerState
	Labels         []byte
	DeploymentName types.Option[string]
}

// Upsert a runner and return the deployment ID that it is assigned to, if any.
// If the deployment name is null, then deployment_rel.id will be null,
// otherwise we try to retrieve the deployments.id using the key. If
// there is no corresponding deployment, then the deployment ID is -1
// and the parent statement will fail due to a foreign key constraint.
func (q *Queries) UpsertRunner(ctx context.Context, arg UpsertRunnerParams) (types.Option[int64], error) {
	row := q.db.QueryRow(ctx, upsertRunner,
		arg.Key,
		arg.Endpoint,
		arg.State,
		arg.Labels,
		arg.DeploymentName,
	)
	var deployment_id types.Option[int64]
	err := row.Scan(&deployment_id)
	return deployment_id, err
}
