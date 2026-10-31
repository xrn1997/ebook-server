-- 0001_init: 当前四个实体的基线 schema（从 GORM AutoMigrate 导出）。
-- 新库直接执行本文件；AutoMigrate 时代的旧库先由 Migrate 收养——收养会把二进制里
-- 内嵌的**全部**版本一次性标记为已应用，不是只标这一个。

CREATE TABLE IF NOT EXISTS `users` (
  `uid` integer PRIMARY KEY AUTOINCREMENT,
  `email` text NOT NULL,
  `password` text NOT NULL,
  `username` text NOT NULL,
  `nickname` text,
  `avatar` text,
  `login_attempts` integer DEFAULT 0,
  `locked_until` datetime,
  `created_at` datetime,
  `updated_at` datetime,
  `deleted_at` datetime
);

CREATE TABLE IF NOT EXISTS `refresh_tokens` (
  `id` integer PRIMARY KEY AUTOINCREMENT,
  `token_hash` text NOT NULL,
  `user_id` integer NOT NULL,
  `expires_at` datetime,
  `created_at` datetime
);

CREATE TABLE IF NOT EXISTS `comments` (
  `id` integer PRIMARY KEY AUTOINCREMENT,
  `user_id` integer NOT NULL,
  `content` text NOT NULL,
  `comment_key` text,
  `chapter_url` text,
  `chapter_name` text,
  `book_name` text,
  `created_at` datetime,
  `deleted_at` datetime,
  CONSTRAINT `fk_comments_user` FOREIGN KEY (`user_id`) REFERENCES `users`(`uid`)
);

CREATE TABLE IF NOT EXISTS `operation_logs` (
  `id` integer PRIMARY KEY AUTOINCREMENT,
  `user_id` integer,
  `username` text,
  `method` text,
  `path` text,
  `ip` text,
  `user_agent` text,
  `response_code` integer,
  `error_code` text,
  `error_message` text,
  `request_body` text,
  `created_at` datetime
);

CREATE UNIQUE INDEX IF NOT EXISTS `idx_users_email` ON `users`(`email`);
CREATE INDEX IF NOT EXISTS `idx_users_deleted_at` ON `users`(`deleted_at`);
CREATE UNIQUE INDEX IF NOT EXISTS `idx_refresh_tokens_token_hash` ON `refresh_tokens`(`token_hash`);
CREATE INDEX IF NOT EXISTS `idx_refresh_tokens_user_id` ON `refresh_tokens`(`user_id`);
CREATE INDEX IF NOT EXISTS `idx_comments_user_id` ON `comments`(`user_id`);
CREATE INDEX IF NOT EXISTS `idx_comments_comment_key` ON `comments`(`comment_key`);
CREATE INDEX IF NOT EXISTS `idx_comments_chapter_url` ON `comments`(`chapter_url`);
CREATE INDEX IF NOT EXISTS `idx_comments_deleted_at` ON `comments`(`deleted_at`);
CREATE INDEX IF NOT EXISTS `idx_operation_logs_user_id` ON `operation_logs`(`user_id`);
CREATE INDEX IF NOT EXISTS `idx_operation_logs_created_at` ON `operation_logs`(`created_at`);
