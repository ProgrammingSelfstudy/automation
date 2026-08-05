CREATE TABLE task (
    id            VARCHAR(36) PRIMARY KEY,
    module_type   VARCHAR(32)  NOT NULL,
    name          VARCHAR(128) NOT NULL,
    status        VARCHAR(16)  NOT NULL DEFAULT 'pending',
    config        JSON         NOT NULL,
    concurrency   INT          NOT NULL,
    total_count   INT          NOT NULL,
    success_count INT          NOT NULL DEFAULT 0,
    fail_count    INT          NOT NULL DEFAULT 0,
    err_msg       TEXT,
    created_by    VARCHAR(64),
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at    DATETIME     NULL,
    finished_at   DATETIME     NULL,
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
);
