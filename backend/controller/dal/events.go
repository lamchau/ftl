package dal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/alecthomas/types/optional"
	"github.com/jackc/pgx/v5"

	"github.com/TBD54566975/ftl/backend/controller/sql"
	"github.com/TBD54566975/ftl/backend/schema"
	"github.com/TBD54566975/ftl/internal/log"
	"github.com/TBD54566975/ftl/internal/model"
)

type EventType = sql.EventType

// Supported event types.
const (
	EventTypeLog               = sql.EventTypeLog
	EventTypeCall              = sql.EventTypeCall
	EventTypeDeploymentCreated = sql.EventTypeDeploymentCreated
	EventTypeDeploymentUpdated = sql.EventTypeDeploymentUpdated
)

// Event types.
//
//sumtype:decl
type Event interface {
	GetID() int64
	event()
}

type LogEvent struct {
	ID             int64
	DeploymentName model.DeploymentName
	RequestName    optional.Option[model.RequestName]
	Time           time.Time
	Level          int32
	Attributes     map[string]string
	Message        string
	Error          optional.Option[string]
	Stack          optional.Option[string]
}

func (e *LogEvent) GetID() int64 { return e.ID }
func (e *LogEvent) event()       {}

type CallEvent struct {
	ID             int64
	DeploymentName model.DeploymentName
	RequestName    optional.Option[model.RequestName]
	Time           time.Time
	SourceVerb     optional.Option[schema.VerbRef]
	DestVerb       schema.VerbRef
	Duration       time.Duration
	Request        []byte
	Response       []byte
	Error          optional.Option[string]
	Stack          optional.Option[string]
}

func (e *CallEvent) GetID() int64 { return e.ID }
func (e *CallEvent) event()       {}

type DeploymentCreatedEvent struct {
	ID                 int64
	DeploymentName     model.DeploymentName
	Time               time.Time
	Language           string
	ModuleName         string
	MinReplicas        int
	ReplacedDeployment optional.Option[model.DeploymentName]
}

func (e *DeploymentCreatedEvent) GetID() int64 { return e.ID }
func (e *DeploymentCreatedEvent) event()       {}

type DeploymentUpdatedEvent struct {
	ID              int64
	DeploymentName  model.DeploymentName
	Time            time.Time
	MinReplicas     int
	PrevMinReplicas int
}

func (e *DeploymentUpdatedEvent) GetID() int64 { return e.ID }
func (e *DeploymentUpdatedEvent) event()       {}

type eventFilterCall struct {
	sourceModule optional.Option[string]
	destModule   string
	destVerb     optional.Option[string]
}

type eventFilter struct {
	level        *log.Level
	calls        []*eventFilterCall
	types        []EventType
	deployments  []model.DeploymentName
	requests     []string
	newerThan    time.Time
	olderThan    time.Time
	idHigherThan int64
	idLowerThan  int64
	descending   bool
}

type EventFilter func(query *eventFilter)

func FilterLogLevel(level log.Level) EventFilter {
	return func(query *eventFilter) {
		query.level = &level
	}
}

// FilterCall filters call events between the given modules.
//
// May be called multiple times.
func FilterCall(sourceModule optional.Option[string], destModule string, destVerb optional.Option[string]) EventFilter {
	return func(query *eventFilter) {
		query.calls = append(query.calls, &eventFilterCall{sourceModule: sourceModule, destModule: destModule, destVerb: destVerb})
	}
}

func FilterDeployments(deploymentNames ...model.DeploymentName) EventFilter {
	return func(query *eventFilter) {
		query.deployments = append(query.deployments, deploymentNames...)
	}
}

func FilterRequests(requestNames ...model.RequestName) EventFilter {
	return func(query *eventFilter) {
		for _, request := range requestNames {
			query.requests = append(query.requests, request.String())
		}
	}
}

func FilterTypes(types ...sql.EventType) EventFilter {
	return func(query *eventFilter) {
		query.types = append(query.types, types...)
	}
}

// FilterTimeRange filters events between the given times, inclusive.
//
// Either maybe be zero to indicate no upper or lower bound.
func FilterTimeRange(olderThan, newerThan time.Time) EventFilter {
	return func(query *eventFilter) {
		query.newerThan = newerThan
		query.olderThan = olderThan
	}
}

// FilterIDRange filters events between the given IDs, inclusive.
func FilterIDRange(higherThan, lowerThan int64) EventFilter {
	return func(query *eventFilter) {
		query.idHigherThan = higherThan
		query.idLowerThan = lowerThan
	}
}

// FilterDescending returns events in descending order.
func FilterDescending() EventFilter {
	return func(query *eventFilter) {
		query.descending = true
	}
}

// The internal JSON payload of a call event.
type eventCallJSON struct {
	DurationMS int64                   `json:"duration_ms"`
	Request    json.RawMessage         `json:"request"`
	Response   json.RawMessage         `json:"response"`
	Error      optional.Option[string] `json:"error,omitempty"`
	Stack      optional.Option[string] `json:"stack,omitempty"`
}

type eventLogJSON struct {
	Message    string                  `json:"message"`
	Attributes map[string]string       `json:"attributes"`
	Error      optional.Option[string] `json:"error,omitempty"`
	Stack      optional.Option[string] `json:"stack,omitempty"`
}

type eventDeploymentCreatedJSON struct {
	MinReplicas        int                                   `json:"min_replicas"`
	ReplacedDeployment optional.Option[model.DeploymentName] `json:"replaced,omitempty"`
}

type eventDeploymentUpdatedJSON struct {
	MinReplicas     int `json:"min_replicas"`
	PrevMinReplicas int `json:"prev_min_replicas"`
}

type eventRow struct {
	sql.Event
	DeploymentName model.DeploymentName
	RequestName    optional.Option[model.RequestName]
}

func (d *DAL) QueryEvents(ctx context.Context, limit int, filters ...EventFilter) ([]Event, error) {
	if limit < 1 {
		return nil, fmt.Errorf("limit must be >= 1, got %d", limit)
	}

	// Build query.
	q := `SELECT e.id AS id,
				e.deployment_id,
				ir.name AS request_name,
				e.time_stamp,
				e.custom_key_1,
				e.custom_key_2,
				e.custom_key_3,
				e.custom_key_4,
				e.type,
				e.payload
			FROM events e
					 LEFT JOIN requests ir on e.request_id = ir.id
			WHERE true -- The "true" is to simplify the ANDs below.
		`

	filter := eventFilter{}
	for _, f := range filters {
		f(&filter)
	}
	var args []any
	index := 1
	param := func(v any) int {
		args = append(args, v)
		index++
		return index - 1
	}
	if !filter.olderThan.IsZero() {
		q += fmt.Sprintf(" AND e.time_stamp <= $%d::TIMESTAMPTZ", param(filter.olderThan))
	}
	if !filter.newerThan.IsZero() {
		q += fmt.Sprintf(" AND e.time_stamp >= $%d::TIMESTAMPTZ", param(filter.newerThan))
	}
	if filter.idHigherThan != 0 {
		q += fmt.Sprintf(" AND e.id >= $%d::BIGINT", param(filter.idHigherThan))
	}
	if filter.idLowerThan != 0 {
		q += fmt.Sprintf(" AND e.id <= $%d::BIGINT", param(filter.idLowerThan))
	}
	deploymentNames := map[int64]model.DeploymentName{}
	// TODO: We should probably make it mandatory to provide at least one deployment.
	deploymentQuery := `SELECT id, name FROM deployments`
	deploymentArgs := []any{}
	if len(filter.deployments) != 0 {
		// Unfortunately, if we use a join here, PG will do a sequential scan on
		// events and deployments, making a 7ms query into a 700ms query.
		// https://www.pgexplain.dev/plan/ecd44488-6060-4ad1-a9b4-49d092c3de81
		deploymentQuery += ` WHERE name = ANY($1::TEXT[])`
		deploymentArgs = append(deploymentArgs, filter.deployments)
	}
	rows, err := d.db.Conn().Query(ctx, deploymentQuery, deploymentArgs...)
	if err != nil {
		return nil, translatePGError(err)
	}
	deploymentIDs := []int64{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		deploymentIDs = append(deploymentIDs, id)
		deploymentNames[id] = model.DeploymentName(name)
	}

	q += fmt.Sprintf(` AND e.deployment_id = ANY($%d::BIGINT[])`, param(deploymentIDs))

	if filter.requests != nil {
		q += fmt.Sprintf(` AND ir.name = ANY($%d::TEXT[])`, param(filter.requests))
	}
	if filter.types != nil {
		// Why are we double casting? Because I hit "cannot find encode plan" and
		// this works around it: https://github.com/jackc/pgx/issues/338#issuecomment-333399112
		q += fmt.Sprintf(` AND e.type = ANY($%d::text[]::event_type[])`, param(filter.types))
	}
	if filter.level != nil {
		q += fmt.Sprintf(" AND (e.type != 'log' OR (e.type = 'log' AND e.custom_key_1::INT >= $%d::INT))\n", param(*filter.level))
	}
	if len(filter.calls) > 0 {
		q += " AND ("
		for i, call := range filter.calls {
			if i > 0 {
				q += " OR "
			}
			if sourceModule, ok := call.sourceModule.Get(); ok {
				q += fmt.Sprintf("(e.type != 'call' OR (e.type = 'call' AND e.custom_key_1 = $%d AND e.custom_key_3 = $%d))", param(sourceModule), param(call.destModule))
			} else if destVerb, ok := call.destVerb.Get(); ok {
				q += fmt.Sprintf("(e.type != 'call' OR (e.type = 'call' AND e.custom_key_3 = $%d AND e.custom_key_4 = $%d))", param(call.destModule), param(destVerb))
			} else {
				q += fmt.Sprintf("(e.type != 'call' OR (e.type = 'call' AND e.custom_key_3 = $%d))", param(call.destModule))
			}
		}
		q += ")\n"
	}

	if filter.descending {
		q += " ORDER BY e.time_stamp DESC, e.id DESC"
	} else {
		q += " ORDER BY e.time_stamp ASC, e.id ASC"
	}

	q += fmt.Sprintf(" LIMIT %d", limit)

	// Issue query.
	rows, err = d.db.Conn().Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", q, translatePGError(err))
	}
	defer rows.Close()

	events, err := transformRowsToEvents(deploymentNames, rows)
	if err != nil {
		return nil, err
	}
	return events, nil
}

func transformRowsToEvents(deploymentNames map[int64]model.DeploymentName, rows pgx.Rows) ([]Event, error) {
	var out []Event
	for rows.Next() {
		row := eventRow{}
		var deploymentID int64
		err := rows.Scan(
			&row.ID, &deploymentID, &row.RequestName, &row.TimeStamp,
			&row.CustomKey1, &row.CustomKey2, &row.CustomKey3, &row.CustomKey4,
			&row.Type, &row.Payload,
		)
		if err != nil {
			return nil, err
		}

		row.DeploymentName = deploymentNames[deploymentID]

		var requestName optional.Option[model.RequestName]
		if key, ok := row.RequestName.Get(); ok {
			requestName = optional.Some(key)
		}
		switch row.Type {
		case sql.EventTypeLog:
			var jsonPayload eventLogJSON
			if err := json.Unmarshal(row.Payload, &jsonPayload); err != nil {
				return nil, err
			}
			level, err := strconv.ParseInt(row.CustomKey1.MustGet(), 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid log level: %q: %w", row.CustomKey1.MustGet(), err)
			}
			out = append(out, &LogEvent{
				ID:             row.ID,
				DeploymentName: row.DeploymentName,
				RequestName:    requestName,
				Time:           row.TimeStamp,
				Level:          int32(level),
				Attributes:     jsonPayload.Attributes,
				Message:        jsonPayload.Message,
				Error:          jsonPayload.Error,
				Stack:          jsonPayload.Stack,
			})

		case sql.EventTypeCall:
			var jsonPayload eventCallJSON
			if err := json.Unmarshal(row.Payload, &jsonPayload); err != nil {
				return nil, err
			}
			var sourceVerb optional.Option[schema.VerbRef]
			sourceModule, smok := row.CustomKey1.Get()
			sourceName, snok := row.CustomKey2.Get()
			if smok && snok {
				sourceVerb = optional.Some(schema.VerbRef{Module: sourceModule, Name: sourceName})
			}
			out = append(out, &CallEvent{
				ID:             row.ID,
				DeploymentName: row.DeploymentName,
				RequestName:    requestName,
				Time:           row.TimeStamp,
				SourceVerb:     sourceVerb,
				DestVerb:       schema.VerbRef{Module: row.CustomKey3.MustGet(), Name: row.CustomKey4.MustGet()},
				Duration:       time.Duration(jsonPayload.DurationMS) * time.Millisecond,
				Request:        jsonPayload.Request,
				Response:       jsonPayload.Response,
				Error:          jsonPayload.Error,
				Stack:          jsonPayload.Stack,
			})

		case sql.EventTypeDeploymentCreated:
			var jsonPayload eventDeploymentCreatedJSON
			if err := json.Unmarshal(row.Payload, &jsonPayload); err != nil {
				return nil, err
			}
			out = append(out, &DeploymentCreatedEvent{
				ID:                 row.ID,
				DeploymentName:     row.DeploymentName,
				Time:               row.TimeStamp,
				Language:           row.CustomKey1.MustGet(),
				ModuleName:         row.CustomKey2.MustGet(),
				MinReplicas:        jsonPayload.MinReplicas,
				ReplacedDeployment: jsonPayload.ReplacedDeployment,
			})

		case sql.EventTypeDeploymentUpdated:
			var jsonPayload eventDeploymentUpdatedJSON
			if err := json.Unmarshal(row.Payload, &jsonPayload); err != nil {
				return nil, err
			}
			out = append(out, &DeploymentUpdatedEvent{
				ID:              row.ID,
				DeploymentName:  row.DeploymentName,
				Time:            row.TimeStamp,
				MinReplicas:     jsonPayload.MinReplicas,
				PrevMinReplicas: jsonPayload.PrevMinReplicas,
			})

		default:
			panic("unknown event type: " + row.Type)
		}
	}
	return out, nil
}
