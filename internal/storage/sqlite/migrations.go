package sqlite

const currentSchemaVersion = 1

const patrolIndexRestoreVersion = 1

const migrationV1 = `
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE COLLATE NOCASE,
    display_name TEXT NOT NULL,
    password_hash BLOB NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('resident_liaison','officer','inspector','operator','admin')),
    active INTEGER NOT NULL CHECK (active IN (0,1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    user_agent TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_expiry_idx ON sessions(user_id, expires_at);
CREATE TABLE forest_sites (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    operator_id TEXT NOT NULL REFERENCES users(id),
    address TEXT NOT NULL,
    timezone TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active','restricted','suspended')),
    latitude REAL NOT NULL DEFAULT 0,
    longitude REAL NOT NULL DEFAULT 0,
    cutoff_minute INTEGER NOT NULL CHECK (cutoff_minute >= 0 AND cutoff_minute < 1440),
    version INTEGER NOT NULL CHECK (version > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE forest_parcels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    contact_user_id TEXT NOT NULL REFERENCES users(id),
    window_count INTEGER NOT NULL CHECK (window_count > 0),
    sensitivity_lux REAL NOT NULL CHECK (sensitivity_lux > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE forest_assets (
    id TEXT PRIMARY KEY,
    forest_site_id TEXT NOT NULL REFERENCES forest_sites(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    row_number INTEGER NOT NULL CHECK (row_number > 0),
    orientation TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0,1)),
    shielded INTEGER NOT NULL CHECK (shielded IN (0,1)),
    angle_degrees REAL NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    updated_at TEXT NOT NULL,
    UNIQUE(forest_site_id, label)
);
CREATE INDEX forest_assets_forest_site_row_idx ON forest_assets(forest_site_id, row_number);
CREATE TABLE forest_cases (
    id TEXT PRIMARY KEY,
    forest_site_id TEXT NOT NULL REFERENCES forest_sites(id),
    forest_parcel_id TEXT NOT NULL REFERENCES forest_parcels(id),
    reporter_id TEXT NOT NULL REFERENCES users(id),
    owner_id TEXT REFERENCES users(id),
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL,
    priority INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 5),
    version INTEGER NOT NULL CHECK (version > 0),
    submitted_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    resolved_at TEXT,
    reopen_until TEXT
);
CREATE INDEX forest_cases_state_idx ON forest_cases(status, priority, submitted_at);
CREATE INDEX forest_cases_scope_idx ON forest_cases(forest_site_id, forest_parcel_id, submitted_at);
CREATE TABLE case_events (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES forest_cases(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL REFERENCES users(id),
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    request_id TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX case_events_case_time_idx ON case_events(case_id, created_at, id);
CREATE TABLE surveys (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES forest_cases(id) ON DELETE CASCADE,
    inspector_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('draft','published','cancelled')),
    started_at TEXT NOT NULL,
    published_at TEXT,
    summary_lux REAL NOT NULL DEFAULT 0,
    version INTEGER NOT NULL CHECK (version > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX surveys_open_case_idx ON surveys(case_id) WHERE status = 'draft';
CREATE TABLE forest_survey_readings (
    id TEXT PRIMARY KEY,
    forest_survey_id TEXT NOT NULL REFERENCES surveys(id) ON DELETE CASCADE,
    position TEXT NOT NULL,
    lux REAL NOT NULL CHECK (lux >= 0),
    measured_at TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    UNIQUE(forest_survey_id, sequence)
);
CREATE TABLE forest_plans (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES forest_cases(id) ON DELETE CASCADE,
    forest_survey_id TEXT NOT NULL REFERENCES surveys(id),
    created_by TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL,
    description TEXT NOT NULL,
    cutoff_minute INTEGER NOT NULL,
    expected_max_lux REAL NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    approved_at TEXT
);
CREATE UNIQUE INDEX mitigation_active_case_idx ON forest_plans(case_id) WHERE status NOT IN ('completed','rejected');
CREATE TABLE forest_asset_actions (
    id TEXT PRIMARY KEY,
    plan_id TEXT NOT NULL REFERENCES forest_plans(id) ON DELETE CASCADE,
    forest_asset_id TEXT NOT NULL REFERENCES forest_assets(id),
    action_type TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','claimed','completed','failed')),
    target_value TEXT NOT NULL,
    worker_id TEXT,
    lease_expires_at TEXT,
    attempt INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL CHECK (version > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(plan_id, forest_asset_id, action_type)
);
CREATE INDEX forest_asset_actions_claim_idx ON forest_asset_actions(status, lease_expires_at, created_at);
CREATE TABLE inspection_rounds (
    id TEXT PRIMARY KEY,
    case_id TEXT NOT NULL REFERENCES forest_cases(id) ON DELETE CASCADE,
    plan_id TEXT NOT NULL REFERENCES forest_plans(id),
    inspector_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('open','passed','failed')),
    observed_max_lux REAL NOT NULL DEFAULT 0,
    resident_agreed INTEGER NOT NULL DEFAULT 0 CHECK (resident_agreed IN (0,1)),
    notes TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL CHECK (version > 0),
    created_at TEXT NOT NULL,
    completed_at TEXT
);
CREATE UNIQUE INDEX verification_open_case_idx ON inspection_rounds(case_id) WHERE status = 'open';
CREATE TABLE outbox_jobs (
    id TEXT PRIMARY KEY,
    topic TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    payload BLOB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','running','done','dead')),
    available_at TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL CHECK (max_attempts > 0),
    worker_id TEXT,
    lease_expires_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX outbox_claim_idx ON outbox_jobs(status, available_at, lease_expires_at);
CREATE TABLE idempotency_keys (
    scope TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    response_code INTEGER NOT NULL,
    response_body BLOB NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY(scope, idempotency_key)
);
CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    result TEXT NOT NULL,
    request_id TEXT NOT NULL,
    metadata_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX audit_object_idx ON audit_events(object_type, object_id, created_at);
`
