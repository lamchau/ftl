-- migrate:up
CREATE
    EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION notify_event() RETURNS TRIGGER AS
$$
DECLARE
    payload JSONB;
BEGIN
    IF TG_OP = 'DELETE' THEN
        payload = jsonb_build_object(
                'table', TG_TABLE_NAME,
                'action', TG_OP,
                'old', old.key
            );
    ELSE
        payload = jsonb_build_object(
                'table', TG_TABLE_NAME,
                'action', TG_OP,
                'new', new.key
            );
    END IF;
    PERFORM pg_notify('notify_events', payload::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE modules
(
    id       BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    language VARCHAR        NOT NULL,
    name     VARCHAR UNIQUE NOT NULL
);

CREATE TABLE deployments
(
    id           BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
    module_id    BIGINT      NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
    -- Unique identifier for this deployment.
    "key"        UUID UNIQUE NOT NULL,
    -- Proto-encoded module schema.
    "schema"     BYTEA       NOT NULL,

    min_replicas INT         NOT NULL DEFAULT 0
);

CREATE INDEX deployments_module_id_idx ON deployments (module_id);
-- Only allow one deployment per module.
CREATE UNIQUE INDEX deployments_unique_idx ON deployments (module_id)
    WHERE min_replicas > 0;

CREATE TRIGGER deployments_notify_event
    AFTER INSERT OR UPDATE OR DELETE
    ON deployments
    FOR EACH ROW
EXECUTE PROCEDURE notify_event();

CREATE TABLE artefacts
(
    id         BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
    -- SHA256 digest of the content.
    digest     BYTEA UNIQUE NOT NULL,
    content    BYTEA        NOT NULL
);

CREATE TABLE deployment_artefacts
(
    artefact_id   BIGINT      NOT NULL REFERENCES artefacts (id) ON DELETE CASCADE,
    deployment_id BIGINT      NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
    executable    BOOLEAN     NOT NULL,
    -- Path relative to the module root.
    path          VARCHAR     NOT NULL
);

CREATE INDEX deployment_artefacts_deployment_id_idx ON deployment_artefacts (deployment_id);

CREATE TABLE deployment_logs
(
    id            BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    deployment_id BIGINT      NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    verb          VARCHAR,
    time_stamp    TIMESTAMPTZ NOT NULL,
    level         INT         NOT NULL, -- https://opentelemetry.io/docs/specs/otel/logs/data-model/#displaying-severity
    scope         VARCHAR     NOT NULL,
    message       TEXT        NOT NULL,
    error         TEXT
);

CREATE TYPE runner_state AS ENUM (
    -- The Runner is available to run deployments.
    'idle',
    -- The Runner is reserved but has not yet deployed.
    'reserved',
    -- The Runner has been assigned a deployment.
    'assigned',
    -- The Runner is dead.
    'dead'
    );

-- Runners are processes that are available to run modules.
CREATE TABLE runners
(
    id                  BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    -- Unique identifier for this runner, generated at startup.
    key                 UUID UNIQUE  NOT NULL,
    created             TIMESTAMPTZ  NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
    last_seen           TIMESTAMPTZ  NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
    -- If the runner is reserved, this is the time at which the reservation expires.
    reservation_timeout TIMESTAMPTZ,
    state               runner_state NOT NULL DEFAULT 'idle',
    language            VARCHAR      NOT NULL,
    endpoint            VARCHAR      NOT NULL,
    deployment_id       BIGINT       REFERENCES deployments (id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX runners_endpoint_not_dead_idx ON runners (endpoint) WHERE state <> 'dead';
CREATE INDEX runners_state_idx ON runners (state);
CREATE INDEX runners_deployment_id_idx ON runners (deployment_id);
CREATE INDEX runners_language_idx ON runners (language);

CREATE TABLE ingress_routes
(
    method        VARCHAR NOT NULL,
    path          VARCHAR NOT NULL,
    -- The deployment that should handle this route.
    deployment_id BIGINT  NOT NULL REFERENCES deployments (id) ON DELETE CASCADE,
    -- Duplicated here to avoid having to join from this to deployments then modules.
    module        VARCHAR NOT NULL,
    verb          VARCHAR NOT NULL
);

CREATE INDEX ingress_routes_method_path_idx ON ingress_routes (method, path);

-- Inbound requests.
CREATE TABLE ingress_requests
(
    id          BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    key         UUID UNIQUE NOT NULL,
    source_addr VARCHAR     NOT NULL
);

CREATE TYPE controller_state AS ENUM (
    'live',
    'dead'
    );


CREATE TABLE controller
(
    id        BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    key       UUID UNIQUE      NOT NULL,
    created   TIMESTAMPTZ      NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
    last_seen TIMESTAMPTZ      NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
    state     controller_state NOT NULL DEFAULT 'live',
    endpoint  VARCHAR          NOT NULL
);

CREATE UNIQUE INDEX controller_endpoint_not_dead_idx ON controller (endpoint) WHERE state <> 'dead';

CREATE TABLE calls
(
    id            BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    request_id    BIGINT      NOT NULL REFERENCES ingress_requests (id) ON DELETE CASCADE,
    runner_id     BIGINT      NOT NULL REFERENCES runners (id) ON DELETE CASCADE,
    controller_id BIGINT      NOT NULL REFERENCES controller (id) ON DELETE CASCADE,
    time          TIMESTAMPTZ NOT NULL DEFAULT (NOW() AT TIME ZONE 'utc'),
    dest_module   VARCHAR     NOT NULL,
    dest_verb     VARCHAR     NOT NULL,
    source_module VARCHAR     NOT NULL,
    source_verb   VARCHAR     NOT NULL,
    duration_ms   BIGINT      NOT NULL,
    request       JSONB       NOT NULL,
    response      JSONB,
    error         TEXT
);

CREATE INDEX calls_duration_ms_idx ON calls (duration_ms);
CREATE INDEX calls_source_module_idx ON calls (source_module);
CREATE INDEX calls_source_verb_idx ON calls (source_verb);
CREATE INDEX calls_dest_module_idx ON calls (dest_module);
CREATE INDEX calls_dest_verb_idx ON calls (dest_verb);

-- migrate:down