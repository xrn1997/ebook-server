package model

import "time"

// shanghaiTZ 固定 UTC+8 时区（Asia/Shanghai 无夏令时，可安全用 FixedZone；
// 避免依赖运行环境的 tzdata，保证任意部署时区一致）。
var shanghaiTZ = time.FixedZone("Asia/Shanghai", 8*60*60)

// FormatShanghaiTime 把时间格式化为上海时区的 "yyyy-MM-dd HH:mm:ss"。
//
// 评论视图的 add_time 契约固定上海时区（ADR-0011），不依赖服务器本地时区，
// 避免部署机时区不同导致客户端解析错乱。
func FormatShanghaiTime(t time.Time) string {
	return t.In(shanghaiTZ).Format("2006-01-02 15:04:05")
}
