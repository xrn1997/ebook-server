/**
 * 测试夹具取位与存在性收口。
 *
 * `noUncheckedIndexedAccess` 把 `arr[i]` 定为 `T | undefined`，而这些数组与 `.find()` 结果
 * 都是测试自己按已知内容构造的（门数组、`findAll` 结果、按钮文本查找）——拿不到就说明夹具
 * 与断言脱节。那时要的是「缺哪一项、共几项」这条信息，而不是深处冒出来的 TypeError，
 * 也不是被 `?.` 或 `!` 折叠过去、下一行断言蒙对的假绿。
 *
 * 只给测试用：生产代码里的同款需求要走各层已有的失败通道（口径见 `ADR-0026`）。这条边界由
 * `eslint.config.js` 的 `no-restricted-imports` 强制——生产文件 import 本模块即红。
 */
export function must<T>(value: T | undefined, what: string): T {
  if (value === undefined) throw new Error(`夹具缺${what}`)
  return value
}

/**
 * 按位置取夹具条目；越界即响亮失败。
 *
 * `what` 是给失败信息加的标签。一个测试文件里有多份夹具数组时（门数组、按钮集合、输入框
 * 集合），光说「缺第 1 项」要读栈才知道是哪一份；同文件只有一份夹具时可省略。
 */
export function pick<T>(items: readonly T[], index: number, what?: string): T {
  return must(items[index], what ? `${what} 的第 ${index} 项（共 ${items.length} 项）` : `第 ${index} 项（共 ${items.length} 项）`)
}
