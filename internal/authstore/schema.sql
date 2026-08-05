CREATE TABLE `user` (
    id             VARCHAR(36) PRIMARY KEY,
    username       VARCHAR(128) NOT NULL UNIQUE,
    password_hash  VARCHAR(255) NOT NULL,
    totp_secret    VARCHAR(64),
    totp_enabled   TINYINT NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE backup_code (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id     VARCHAR(36) NOT NULL,
    code_hash   VARCHAR(255) NOT NULL,
    used_at     DATETIME NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user (user_id)
);

CREATE TABLE `session` (
    id          VARCHAR(64) PRIMARY KEY,
    user_id     VARCHAR(36) NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  DATETIME NOT NULL,
    INDEX idx_user (user_id),
    INDEX idx_expires (expires_at)
);

CREATE TABLE login_attempt (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    ip          VARCHAR(64) NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_ip_created (ip, created_at)
);
