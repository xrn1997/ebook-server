#!/usr/bin/env bash
# 清理过期的 operation_logs，避免审计表在持续流量/攻击下把磁盘写满。
#
# 默认保留 30 天，可用 AUDIT_RETENTION_DAYS 覆盖（必须是 ≥1 的整数；0 不是
# 「关闭」——「datetime('now','-0 days')」会把全部历史当过期删光，所以直接拒绝。
# 要停用清理，禁用 timer：sudo systemctl disable --now ebook-audit-prune.timer）。
#
# 时间比较两侧都套 datetime()：created_at 由 glebarez 驱动写成带时区偏移的串
# （2026-11-17 08:00:00+08:00，可能带小数秒），而 datetime('now') 是 UTC 无时区
# 后缀格式；裸字符串比较会把时区偏移与小数秒算进大小，cutoff 边界上的记录会
# 删错整天。datetime() 把两侧归一到同一 UTC 形态再比。
#
# SQLite 的 DELETE 不会立刻归还文件空间；WAL 模式下空间会被后续写入复用，
# 不需要每次 VACUUM（VACUUM 会锁库）。
set -euo pipefail

DB="${1:-/opt/ebook-server/ebook.db}"
DAYS="${AUDIT_RETENTION_DAYS:-30}"

case "$DAYS" in
    ''|*[!0-9]*)
        echo "AUDIT_RETENTION_DAYS 必须是十进制正整数，got: $DAYS" >&2
        exit 1
        ;;
esac
if (( 10#$DAYS < 1 )); then
    echo "AUDIT_RETENTION_DAYS 必须 ≥ 1（0 会删光全部审计记录）" >&2
    exit 1
fi

if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "未找到 sqlite3；Debian/Ubuntu: sudo apt-get install -y sqlite3" >&2
    exit 1
fi

if [[ ! -f "$DB" ]]; then
    echo "数据库不存在: $DB" >&2
    exit 1
fi

# busy_timeout 与 backend/pkg/database 的 busyTimeoutMS 对齐：清理期间遇到应用写入时
# 等待而不是立刻报锁。
sqlite3 "$DB" "PRAGMA busy_timeout=5000; DELETE FROM operation_logs WHERE datetime(created_at) < datetime('now', '-${DAYS} days');"
echo "已清理 $DB 中超过 ${DAYS} 天的 operation_logs。"
