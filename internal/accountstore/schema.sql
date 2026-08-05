CREATE TABLE account (
    id          VARCHAR(36) PRIMARY KEY,
    group_id    VARCHAR(36)  NOT NULL,
    username    VARCHAR(128) NOT NULL,
    password    VARCHAR(512) NOT NULL,
    extra       JSON,
    enabled     TINYINT      NOT NULL DEFAULT 1,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_group (group_id)
);
