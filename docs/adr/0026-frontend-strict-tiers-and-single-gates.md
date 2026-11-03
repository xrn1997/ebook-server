# 前端编译器档位与门禁收成一条 make gates

Status: accepted

后台前端此前没有任何 lint 工具链，tsconfig 只有 `strict` 与 `isolatedModules`；本仓也**没有 CI**——四道门（`go vet`、`go test`、前端类型检查、前端测试）只住在 Makefile 里，靠人记得跑。判据很清楚：门禁不由 CI 强制执行，就等于不存在。

## 决定

- **前端 tsconfig 补满**：`noUncheckedIndexedAccess`、`exactOptionalPropertyTypes`、`erasableSyntaxOnly`、`verbatimModuleSyntax`、`noImplicitOverride`、`noFallthroughCasesInSwitch`、未用变量与参数。`useDefineForClassFields` 原本就显式写着，保持——它与 `target: ES2022` 隐式打开的值一致，写出来是为了让类字段语义在配置里现查得到。
- **从零建 ESLint**：走官方 `withVueTs` + `vueTsConfigs.strictTypeChecked`，不手搭 `parserOptions`。
  同时把 `allowComponentTypeUnsafety` 显式取 `false`：默认值 `true` 会推入一条覆盖全部 TS 与单文件
  组件的配置，把 `no-unsafe-*` 五类一起关掉——「lint 管 any 扩散」那句话于是落空。代价只落在真正
  操作组件类型的 `src/main.ts` 与 `src/router.ts`，由两处带理由的作用域豁免承担，其余文件五类全在线。
- **新增 `frontend-deps`、`frontend-lint` 与 `gates` 目标**。`gates` 由哪些目标组成**只写在
  `Makefile` 与 `docs/development.md` 的表里**，本 ADR 不重抄：这条命令被抄进七个家、漂了四处，
  其中一处就是本篇自己。
- **新增 `.github/workflows/ci.yml`，唯一动作是 `make gates`**。

## 被否方案

- **让 `allowComponentTypeUnsafety` 停在默认的 `true`**：它推入的那条关规则配置覆盖全部 TS 与单文件
  组件，为了入口与路由那两个文件的工具链缺口，把整个仓生产代码的 `no-unsafe-*` 一起灭掉。
  取「显式 `false` + 只在这两个文件豁免真正触发的两条规则」。
- **手写 `parserOptions.parser` / `extraFileExtensions`**：单文件组件的类型面进不了 ESLint 的 TS program，路由与入口文件里对每个组件的 import 都会变成 error 类型，压出一批假的 `no-unsafe-*`。那是工具链缺口，不是代码缺陷。
- **`eslint-plugin-vue` 的 `flat/recommended`**：带进一批纯排版规则（`max-attributes-per-line`、`html-self-closing` 等），与「ESLint 不做格式」的分工冲突。取 `flat/essential`，只收正确性层。
- **引入格式化器**：本仓没有，引入会产生一次全仓 reformat 的巨型 diff，与本次目的无关；格式主张仍靠既有排版与评审。
- **只补 CI 不补 lint**：那会让「门」少一扇。本轮 `src/api.ts` 的两处真问题（`fetch` 的 `init` 把 `body` 以显式 `undefined` 传入、信封解包把 `T | undefined` 当 `T`）都由编译器抓：前者是新开的 `exactOptionalPropertyTypes`，后者是 `strict` 原本就带的 `strictNullChecks`；lint 管的是 `vue-tsc` 不报的另一类——`any` 从边界扩散、悬浮 Promise、非空断言。两扇门不是一个判据。

## 后果

- `noUncheckedIndexedAccess` 逼出来的取位全部落在测试文件（`list[0]` 这类断言取位），生产代码为 0；这批统一收口到 `src/fixture.ts` 的 `pick` / `must`，越界即抛「夹具缺第几项、共几项」。同一文件里有多份夹具时用 `pick` 的第三个参数写明是哪一份。
- `fixture` 只给测试用这条边界由 ESLint 的 `no-restricted-imports` 强制（生产文件 import 它即红），不靠注释自律——判据就是本篇开头那句：不由机器执行的门等于不存在。
- `no-non-null-assertion` 为 error，测试里原有的 `find(...)!` 同批收口到 `pick` / `must`。
- 八条档位里今天真正咬到代码的是 `exactOptionalPropertyTypes`（`src/api.ts` 那处显式 `undefined`）与 `noUncheckedIndexedAccess`（测试取位）；`erasableSyntaxOnly`、`noImplicitOverride`、`noFallthroughCasesInSwitch` 与未用变量/参数是前瞻性的——本仓当前没有 `enum`、没有类字段覆盖、没有 `switch` 落空点，它们守的是以后写进来的那一个。
- 三个前端门共用一条 `frontend-deps` 依赖边（戳文件 `frontend/node_modules/.package-lock.json`）：此前各自 `test -d node_modules || npm ci`，`make -j` 下会并发跑两个 `npm ci` 写同一棵树。
- 新增 lint 门把前端工具链的 Node 下限抬到 ESLint 10 的要求（`^20.19.0 || ^22.13.0 || >=24`），`frontend/package.json` 的 `engines` 与 `README.md` 的环境要求同批改写。没开 `engine-strict`：依赖树里 `nopt`/`abbrev` 声明的下限比这更高，开了它连声明支持的那档都装不上；下限之下的真实失败点是 ESLint 自己起不来，报得足够响。CI 只在 Node 24 上跑，没有替下限补一条作业。
- `vue/multi-word-component-names` 关闭：视图按路由挂载、不作标签嵌套，规则的原生元素碰撞理由在本仓不成立，而改名会连带改测试与文档引用。
- CI 与发版不存在两套清单：本地跑 `make gates` 与 CI 跑的是同一条。

锚点：`frontend/tsconfig.json`；`frontend/eslint.config.js`；`frontend/src/fixture.ts` 的 `must` / `pick`；`frontend/src/api.ts` 的 `isEnvelope`；`frontend/src/requestGuard.ts`；`frontend/src/useAdminDetail.ts`；`frontend/src/views/AdminViews.test.ts`；`Makefile` 的 `gates` / `frontend-lint` / `frontend-deps`；`.github/workflows/ci.yml`。
