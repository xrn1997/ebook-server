/**
 * vitest 全局 setup：为每条测试文件装一个全新的内存 localStorage。
 *
 * `src/api.ts` 的令牌是「每次读写都现取浏览器全局」的，所以替身必须在模块导入之前就
 * 挂在 globalThis 上。vitest 2.x 的 happy-dom 环境并不保证把 `localStorage` 复制成全局
 * （window 上有、全局没有），把测试的成败押在环境实现细节上不如自己钉住：这里装好，
 * 视图与 api 层的令牌链路在所有环境下走同一条代码路径。
 */

function memoryStorage(): Storage {
  const map = new Map<string, string>()
  return {
    get length(): number {
      return map.size
    },
    key: (index: number) => [...map.keys()][index] ?? null,
    getItem: (name: string) => map.get(name) ?? null,
    setItem: (name: string, value: string) => {
      map.set(name, String(value))
    },
    removeItem: (name: string) => {
      map.delete(name)
    },
    clear: () => {
      map.clear()
    },
  } as Storage
}

Object.defineProperty(globalThis, 'localStorage', {
  value: memoryStorage(),
  configurable: true,
  writable: true,
})
