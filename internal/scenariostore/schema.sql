CREATE TABLE scenario (
    id          VARCHAR(36) PRIMARY KEY,
    name        VARCHAR(128) NOT NULL,
    definition  JSON         NOT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);
