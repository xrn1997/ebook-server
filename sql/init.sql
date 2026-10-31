-- =========================================================
-- init.sql — MySQL 建表参考脚本
--
-- 项目实际使用 SQLite。生产启动时由 backend/pkg/database 的 Migrate
-- 执行 backend/migrations/*.sql 建表（ADR-0023），无需手动执行本脚本；
-- 测试库才走 AutoMigrate，它不是生产迁移路径。
--
-- 本文件**不是 schema 的事实源**，只是把当前结构翻成 MySQL 方言的参考，
-- 供将来真的迁往 MySQL 或 DBA 审阅时使用。事实源有两处：
--   - 字段与索引：backend/model 下的结构体
--   - 实际建表语句：backend/migrations/*.sql
--
-- 两处结构变更时本脚本要跟着改；本脚本被改过不代表结构变过——以事实源为准。
-- （上次同步：与 backend/migrations/0001_init.sql 对齐，列集合与索引名一致。）
-- =========================================================

-- 创建数据库
CREATE DATABASE IF NOT EXISTS ebook DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE ebook;

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    uid BIGINT PRIMARY KEY AUTO_INCREMENT,
    email VARCHAR(100) NOT NULL COMMENT '邮箱（登录主标识）',
    password VARCHAR(255) NOT NULL COMMENT '密码',
    username VARCHAR(50) NOT NULL COMMENT '用户名（可重复）',
    nickname VARCHAR(50) COMMENT '昵称',
    avatar VARCHAR(255) COMMENT '头像',
    login_attempts INT DEFAULT 0 COMMENT '连续登录失败次数',
    locked_until TIMESTAMP NULL COMMENT '登录锁定截止时间',
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    deleted_at TIMESTAMP NULL COMMENT '删除时间',
    UNIQUE INDEX idx_users_email (email),
    INDEX idx_users_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';

-- 刷新令牌表
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    token_hash VARCHAR(64) NOT NULL COMMENT 'refresh token 的 SHA-256 哈希',
    user_id BIGINT NOT NULL COMMENT '用户ID',
    expires_at TIMESTAMP NULL COMMENT '过期时间',
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    UNIQUE INDEX idx_refresh_tokens_token_hash (token_hash),
    INDEX idx_refresh_tokens_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='刷新令牌表';

-- 评论表
CREATE TABLE IF NOT EXISTS comments (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL COMMENT '用户ID',
    content TEXT NOT NULL COMMENT '评论内容',
    comment_key VARCHAR(255) COMMENT '客户端派生的评论聚合键（ADR-0014）',
    chapter_url VARCHAR(500) COMMENT '章节地址（兼容/展示用）',
    chapter_name VARCHAR(255) COMMENT '章节名（展示用）',
    book_name VARCHAR(255) COMMENT '书名（展示用）',
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    deleted_at TIMESTAMP NULL COMMENT '删除时间',
    INDEX idx_comments_user_id (user_id),
    INDEX idx_comments_comment_key (comment_key),
    INDEX idx_comments_chapter_url (chapter_url),
    INDEX idx_comments_deleted_at (deleted_at),
    CONSTRAINT fk_comments_user FOREIGN KEY (user_id) REFERENCES users(uid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='评论表';

-- 操作日志表
CREATE TABLE IF NOT EXISTS operation_logs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT COMMENT '用户ID',
    username VARCHAR(50) COMMENT '用户名',
    method VARCHAR(10) COMMENT '请求方法',
    path VARCHAR(255) COMMENT '请求路径',
    ip VARCHAR(50) COMMENT 'IP地址',
    user_agent TEXT COMMENT 'User-Agent',
    response_code INT COMMENT '响应码',
    error_code VARCHAR(10) COMMENT '业务错误码',
    error_message TEXT COMMENT '业务错误文案',
    request_body TEXT COMMENT '请求体（不记录：ADR-0018）',
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_operation_logs_user_id (user_id),
    INDEX idx_operation_logs_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='操作日志表';
