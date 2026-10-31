// Package migrations 嵌入版本化迁移 SQL。
//
// 文件名即版本号（NNNN_slug.sql）；迁移顺序由文件名排序决定。SQL 由
// pkg/database.Migrate 执行，本包不依赖 model 或数据库连接。
package migrations

import "embed"

// FS 包含本目录下的全部迁移 SQL。
//
//go:embed *.sql
var FS embed.FS
