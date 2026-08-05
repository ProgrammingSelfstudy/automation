CREATE TABLE task_result (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    task_id        VARCHAR(36) NOT NULL,
    account_id     VARCHAR(36) NOT NULL,
    account_name   VARCHAR(128) NOT NULL,
    seq_no         INT NOT NULL,
    steps          JSON,
    formula_result DOUBLE,
    success        TINYINT NOT NULL,
    err_msg        TEXT,
    cost_ms        BIGINT,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_account (task_id, account_id, seq_no)
);
