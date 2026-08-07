CREATE TABLE perf_task (
    id                  VARCHAR(36) PRIMARY KEY,
    user_id             VARCHAR(36) NOT NULL,
    device_id           VARCHAR(128) NOT NULL,
    package_name        VARCHAR(255) NOT NULL,
    process_name        VARCHAR(255) NOT NULL,
    platform            VARCHAR(16) NOT NULL,
    device_model        VARCHAR(64),
    status               VARCHAR(16) NOT NULL,
    start_time          DATETIME NOT NULL,
    stop_time           DATETIME NOT NULL,
    sample_interval_ms  BIGINT NOT NULL,
    sample_count        INT NOT NULL,
    last_error          TEXT,
    samples             JSON NOT NULL,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user (user_id)
);
