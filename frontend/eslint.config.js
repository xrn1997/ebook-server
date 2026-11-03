import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import { withVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'

/**
 * 后台前端的机器门。
 *
 * 分工：`vue-tsc` 管类型形状与可赋值性（含 `noUncheckedIndexedAccess`、
 * `exactOptionalPropertyTypes` 这些 tsconfig 档位），ESLint 管 tsc 看不见的那一类——
 * 显式 `any`、来自边界的 `any` 扩散、悬浮 Promise、非空断言。所以要用**类型感知**规则集：
 * `no-unsafe-*` 拿不到类型信息就没有意义。
 *
 * 类型感知配置走官方 `withVueTs` + `vueTsConfigs`，不手搭 parserOptions：手写时 `.vue` 的
 * 类型面进不了 TS program，`src/router.ts` 与 `src/main.ts` 里每一个 `import './views/X.vue'`
 * 都会被当成 error 类型，压出一批假的 `no-unsafe-assignment`——那是工具链缺口，不是缺陷。
 *
 * 不写格式类规则：仓内没有格式化工具，引一个会造成全仓 reformat 的巨型 diff；ESLint 一旦代替
 * 格式化工具立规就成了和现有排版打架的那一方（`flat/recommended` 带进 `max-attributes-per-line`、
 * `html-self-closing` 这类纯排版规则，所以取 `flat/essential`，只收正确性层）。
 */

/**
 * 生产代码的两条 import 边界，由下面两个块共用同一份定义。
 *
 * 提成常量不是为省事：扁平配置里同一条规则由后面匹配到的块**整体覆盖** options，
 * 视图那一块若只写自己新增的那条，就会把 fixture 那条边界从 `.vue` 文件上悄悄关掉。
 */
const fixtureImport = {
  group: ['**/fixture', '**/fixture.js'],
  message: 'fixture 的 must/pick 只给测试用；生产代码走各层已有的失败通道。',
}

const successCodeImport = {
  group: ['**/api'],
  importNames: ['SUCCESS_CODE'],
  message: '视图只看 unwrap 的 ok/message，不再自己比较业务码（见 api.ts 头注释与 ADR-0026）。',
}

export default withVueTs(
  // `allowComponentTypeUnsafety` 默认 true，而它推入的覆盖配置 `files` 是
  // `**/*.ts`/`**/*.tsx`/`**/*.mts`/`**/*.vue`——不只 `.vue`。留在这个默认值上，
  // `no-unsafe-*` 五类在全部生产代码里都是关的，本仓承诺的「边界 any 扩散」门等于不存在。
  // 取 false 打开它，代价只落在真正操作组件类型的 `src/main.ts` 与 `src/router.ts`，
  // 由下面那条带理由的作用域豁免承担。
  { allowComponentTypeUnsafety: false },

  { ignores: ['dist/**', 'node_modules/**', 'coverage/**'] },

  js.configs.recommended,
  ...pluginVue.configs['flat/essential'],
  vueTsConfigs.strictTypeChecked,

  {
    rules: {
      // 数字插值是本仓的既有写法：`HTTP ${res.status}`、`/users/${uid}`、`/comments/${id}`。
      // strict 档把 allowNumber 关掉，代价是把这些正当写法改成 `String(x)` 噪音。
      '@typescript-eslint/restrict-template-expressions': ['error', { allowNumber: true }],

      // 视图按路由挂载（`src/router.ts`），不作标签嵌套；这条规则的立规理由（与原生元素同名）
      // 在这里不成立，而改名会连带改测试与文档引用，零缺陷收益。
      'vue/multi-word-component-names': 'off',

      // 下面两条已由 strictTypeChecked 隐含打开；点名写出来是为了让「本仓的门」在配置里
      // 现查得到，而不是靠读者记住某个预设集包含哪些规则。
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-non-null-assertion': 'error',
    },
  },

  {
    // 这两个文件把 `.vue` 组件喂给 `createApp` / `createRouter`。TS-ESLint 取不到 `.vue` 的
    // 完整类型面，回落到带 `any` 的兜底类型，于是每一条组件 import 都报「error typed」——
    // 工具链缺口，不是缺陷（与上面否掉手搭 parserOptions 同一个理由）。
    // 豁免只到真正触发的两条规则、只在这两个文件：`no-unsafe-call`/`member-access`/`return`
    // 在这里仍然在线，其余所有文件由 `no-unsafe-*` 五类守住。
    files: ['src/main.ts', 'src/router.ts'],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
    },
  },

  {
    // `src/fixture.ts` 的 `must`/`pick` 抛的是「夹具缺第几项」这类测试专用失败，没有可展示的
    // 文案、也不接任何失败通道；生产代码引进来等于把契约漂移变成一次崩溃。
    // 这条边界光靠注释不算立过：判据同 ADR-0026 的开头那句——不由机器强制执行的门等于不存在。
    files: ['**/*.ts', '**/*.vue'],
    ignores: ['**/*.test.ts', 'test/**'],
    rules: {
      'no-restricted-imports': ['error', { patterns: [fixtureImport] }],
    },
  },

  {
    // 「视图不得 import `SUCCESS_CODE`」（`src/api.ts` 头注释）同样不能被注释独自守着：
    // 视图只看 `unwrap` 的 ok/message，自己比较业务码就是绕过解包层。判据同上。
    //
    // `Logs.vue` 的 `codeClass` 按业务码归类徽标，是那条注释点名的已知例外，口径见
    // `docs/development.md`「已知不做的事」；那里归完类就该把这条 ignores 一起删掉。
    files: ['src/views/**/*.vue'],
    ignores: ['src/views/Logs.vue'],
    rules: {
      'no-restricted-imports': ['error', { patterns: [fixtureImport, successCodeImport] }],
    },
  },

  {
    // 测试替身里 `vi.fn(async (…) => new Response(…))` 的 `async` 就是为了回一个 Promise，
    // 函数体内没有 await 是正确形态。生产代码保持这条规则在线。
    files: ['**/*.test.ts'],
    rules: { '@typescript-eslint/require-await': 'off' },
  },
)
