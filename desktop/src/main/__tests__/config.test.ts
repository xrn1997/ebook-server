import { describe, it, expect } from 'vitest'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import {
  deriveEndpoints,
  fillMaskedSecrets,
  loadEnvFile,
  maskSensitiveFields,
  mergeConfigToYaml,
  parseEnv,
  readFullConfig,
  resolveConnectHost,
  sanitizeConfig,
  serializeEnv,
  splitConfigToEnv,
  trySanitize,
  validateConfig,
  writeFullConfig,
  type SanitizeResult,
} from '../config'
import type { AppConfig } from '../config'

/** 一份合法配置基线：各用例只覆盖自己关心的字段 */
function validConfig(): AppConfig {
  return {
    server: { port: 9090, mode: 'debug' },
    database: { path: 'ebook.db' },
    jwt: { secret: 'my-jwt-secret', expire_min: 120 },
    smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', password: 'p', from: 'a@qq.com', insecure: false },
    admin: { username: 'a', password: 'p', jwt_secret: 'j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
    upload: { dir: 'uploads' },
    api_docs: { enabled: false },
  }
}

/** 从可辨识联合里取错误列表（非错误分支返回空数组） */
function errorsOf(result: SanitizeResult): string[] {
  return 'errors' in result ? result.errors : []
}

/** 从可辨识联合里取配置（错误分支返回 null） */
function configOf(result: SanitizeResult): AppConfig | null {
  return 'config' in result ? result.config : null
}

describe('parseEnv', () => {
  it('解析 KEY=VALUE 格式', () => {
    const result = parseEnv('SMTP_PASSWORD=abc123\nJWT_SECRET=xyz')
    expect(result).toEqual({ SMTP_PASSWORD: 'abc123', JWT_SECRET: 'xyz' })
  })

  it('忽略空行和注释行', () => {
    const result = parseEnv('# comment\n\nKEY=val\n')
    expect(result).toEqual({ KEY: 'val' })
  })

  it('处理空字符串', () => {
    const result = parseEnv('')
    expect(result).toEqual({})
  })

  it('处理带引号的值', () => {
    const result = parseEnv('KEY="hello world"')
    expect(result).toEqual({ KEY: 'hello world' })
  })
})

describe('serializeEnv', () => {
  it('序列化为 KEY=VALUE 格式', () => {
    const result = serializeEnv({ SMTP_PASSWORD: 'abc', JWT_SECRET: 'xyz' })
    expect(result).toContain('SMTP_PASSWORD=abc')
    expect(result).toContain('JWT_SECRET=xyz')
  })

  it('空对象返回空字符串', () => {
    expect(serializeEnv({})).toBe('')
  })
})

describe('splitConfigToEnv', () => {
  it('将敏感字段提取到 env，其余保留在 yaml', () => {
    const config: AppConfig = {
      server: { port: 9090, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { secret: 'my-jwt-secret', expire_min: 120 },
      smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', password: 'smtp-pwd', from: 'a@qq.com', insecure: false },
      admin: { username: 'admin', password: 'admin-pwd', jwt_secret: 'admin-jwt', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    const { yamlConfig, envVars } = splitConfigToEnv(config)
    const yaml = yamlConfig as Record<string, any>
    expect(yaml.smtp.password).toBeUndefined()
    expect(yaml.admin.password).toBeUndefined()
    expect(yaml.admin.jwt_secret).toBeUndefined()
    expect(yaml.jwt.secret).toBeUndefined()
    expect(envVars.SMTP_PASSWORD).toBe('smtp-pwd')
    expect(envVars.ADMIN_PASSWORD).toBe('admin-pwd')
    expect(envVars.ADMIN_JWT_SECRET).toBe('admin-jwt')
    expect(envVars.JWT_SECRET).toBe('my-jwt-secret')
  })
})

describe('mergeConfigToYaml', () => {
  it('将 env 变量合并回 config 对象', () => {
    const yamlConfig = {
      server: { port: 9090, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { expire_min: 120 },
      smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', from: 'a@qq.com', insecure: false },
      admin: { username: 'admin', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    const envVars = {
      JWT_SECRET: 'jwt-s',
      SMTP_PASSWORD: 'smtp-s',
      ADMIN_PASSWORD: 'adm-p',
      ADMIN_JWT_SECRET: 'adm-j',
    }
    const merged = mergeConfigToYaml(yamlConfig, envVars)
    expect(merged.jwt.secret).toBe('jwt-s')
    expect(merged.smtp.password).toBe('smtp-s')
    expect(merged.admin.password).toBe('adm-p')
    expect(merged.admin.jwt_secret).toBe('adm-j')
  })
})

describe('validateConfig', () => {
  it('端口号超出范围返回错误', () => {
    const config: AppConfig = {
      server: { port: 99999, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { secret: 's', expire_min: 120 },
      smtp: { host: '', port: 0, username: '', password: '', from: '', insecure: false },
      admin: { username: 'a', password: 'p', jwt_secret: 'j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    const errors = validateConfig(config)
    expect(errors).toContain('server.port 必须在 1-65535 之间')
  })

  it('JWT secret 为空返回错误', () => {
    const config: AppConfig = {
      server: { port: 9090, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { secret: '', expire_min: 120 },
      smtp: { host: '', port: 0, username: '', password: '', from: '', insecure: false },
      admin: { username: 'a', password: 'p', jwt_secret: 'j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    const errors = validateConfig(config)
    expect(errors).toContain('jwt.secret 不能为空')
  })

  it('合法配置返回空错误列表', () => {
    const config: AppConfig = {
      server: { port: 9090, mode: 'debug' },
      database: { path: 'ebook.db' },
      jwt: { secret: 'valid', expire_min: 120 },
      smtp: { host: 'smtp.qq.com', port: 465, username: 'a@qq.com', password: 'p', from: 'a@qq.com', insecure: false },
      admin: { username: 'a', password: 'p', jwt_secret: 'j', expire_min: 60, listen_addr: '127.0.0.1', listen_port: 9091 },
      upload: { dir: 'uploads' },
      api_docs: { enabled: false },
    }
    expect(validateConfig(config)).toEqual([])
  })

  it('后台监听地址为通配时拒绝保存（ADR-0010 网络隔离），包括易漏的 [::] 写法', () => {
    for (const addr of ['0.0.0.0', '::', '[::]', '*']) {
      const errors = validateConfig({ ...validConfig(), admin: { ...validConfig().admin, listen_addr: addr } })
      expect(errors.join(' '), `listen_addr=${addr} 应被拒绝`).toContain('通配地址')
    }
  })

  it('后台监听地址为具体内网 IP 时允许（文档给出的局域网用法）', () => {
    expect(validateConfig({ ...validConfig(), admin: { ...validConfig().admin, listen_addr: '192.168.1.10' } })).toEqual([])
  })

  it('后台监听地址为主机名时拒绝（isIP 校验），localhost 与留空除外', () => {
    const errors = validateConfig({
      ...validConfig(),
      admin: { ...validConfig().admin, listen_addr: 'my-host.local' },
    })
    expect(errors.join(' ')).toContain('listen_addr')
    expect(
      validateConfig({ ...validConfig(), admin: { ...validConfig().admin, listen_addr: 'localhost' } }),
    ).toEqual([])
    expect(
      validateConfig({ ...validConfig(), admin: { ...validConfig().admin, listen_addr: '' } }),
    ).toEqual([])
  })
})

describe('sanitizeConfig', () => {
  it('非对象输入给出结构性错误', () => {
    expect(errorsOf(sanitizeConfig(null))[0]).toContain('对象')
    expect(errorsOf(sanitizeConfig('nope'))[0]).toContain('对象')
  })

  it('缺失配置段与段内缺失键都被点名', () => {
    const errors = errorsOf(sanitizeConfig({
      ...validConfig(),
      smtp: undefined,
      jwt: { secret: 's' },
    }))
    expect(errors).toContain('缺少配置段 smtp')
    expect(errors).toContain('jwt.expire_min 缺失')
  })

  it('数字字段收到字符串时报类型错误而不是静默落盘', () => {
    const errors = errorsOf(sanitizeConfig({ ...validConfig(), server: { port: '', mode: 'debug' } }))
    expect(errors.join(' ')).toContain('server.port 类型应为 number')
  })

  it('renderer 传来的未知键不会进入落盘对象', () => {
    const injected = { ...validConfig(), evil: { rm: 'everything' }, server: { ...validConfig().server, backdoor: true } }
    const config = configOf(sanitizeConfig(injected))
    expect(config).not.toBeNull()
    expect(config).not.toHaveProperty('evil')
    expect(config?.server).not.toHaveProperty('backdoor')
  })

  it('trySanitize 形状不合时返回 null', () => {
    expect(trySanitize({ server: { port: 1 } })).toBeNull()
    expect(trySanitize(validConfig())).toEqual(validConfig())
  })
})

describe('resolveConnectHost', () => {
  it('通配监听地址换算成回环地址', () => {
    for (const addr of ['0.0.0.0', '::', '[::]', '*', '']) {
      expect(resolveConnectHost(addr)).toBe('127.0.0.1')
    }
  })

  it('具体地址原样使用（绑定到内网 IP 时回环并不通）', () => {
    expect(resolveConnectHost('192.168.1.10')).toBe('192.168.1.10')
    expect(resolveConnectHost('localhost')).toBe('localhost')
  })
})

describe('deriveEndpoints', () => {
  it('端口与后台地址都来自配置，数据库路径相对工作目录', () => {
    const endpoints = deriveEndpoints(
      { ...validConfig(), server: { port: 8080, mode: 'release' }, admin: { ...validConfig().admin, listen_addr: '0.0.0.0', listen_port: 9191 }, database: { path: 'ebook.db' } },
      '/work/dir',
    )
    expect(endpoints.apiPort).toBe(8080)
    expect(endpoints.adminBaseUrl).toBe('http://127.0.0.1:9191/admin/api')
    expect(endpoints.dbPath).toBe(path.resolve('/work/dir', 'ebook.db'))
    expect(endpoints.apiBaseUrl).toBe('http://127.0.0.1:8080')
    expect(endpoints.adminOrigin).toBe('http://127.0.0.1:9191')
  })
})

describe('writeFullConfig / readFullConfig 往返', () => {
  it('敏感字段只落在 .env，重新读取后完整还原', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop-config-'))
    const yamlPath = path.join(dir, 'config.yaml')
    const envPath = path.join(dir, '.env')

    writeFullConfig(yamlPath, envPath, {
      ...validConfig(),
      smtp: { ...validConfig().smtp, password: 'smtp-secret' },
    })

    // 磁盘上的 YAML 不应含任何密钥明文
    const onDisk = fs.readFileSync(yamlPath, 'utf-8')
    expect(onDisk).not.toContain('smtp-secret')
    expect(onDisk).not.toContain('my-jwt-secret')
    expect(fs.readFileSync(envPath, 'utf-8')).toContain('SMTP_PASSWORD=smtp-secret')

    const roundTrip = readFullConfig(yamlPath, envPath)
    expect(roundTrip.smtp.password).toBe('smtp-secret')
    expect(roundTrip.jwt.secret).toBe('my-jwt-secret')
    expect(trySanitize(roundTrip)).not.toBeNull()
  })

  it('保存用户自行加入的 .env 键不被抹掉', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'desktop-config-'))
    const yamlPath = path.join(dir, 'config.yaml')
    const envPath = path.join(dir, '.env')
    fs.writeFileSync(envPath, 'MY_OWN_VAR=keep-me\n', 'utf-8')

    writeFullConfig(yamlPath, envPath, validConfig())

    const env = loadEnvFile(envPath)
    expect(env.MY_OWN_VAR).toBe('keep-me')
    expect(env.JWT_SECRET).toBe('my-jwt-secret')
  })
})

describe('敏感值掩码（读路径 mask / 写路径 fill）', () => {
  it('maskSensitiveFields 把四个敏感字段替换为空串，其余字段原样', () => {
    const masked = maskSensitiveFields(validConfig())
    expect(masked.jwt.secret).toBe('')
    expect(masked.smtp.password).toBe('')
    expect(masked.admin.password).toBe('')
    expect(masked.admin.jwt_secret).toBe('')
    expect(masked.jwt.expire_min).toBe(120)
    expect(masked.admin.listen_addr).toBe('127.0.0.1')
  })

  it('maskSensitiveFields 返回深拷贝，不改入参', () => {
    const source = validConfig()
    maskSensitiveFields(source)
    expect(source.jwt.secret).toBe('my-jwt-secret')
  })

  it('fillMaskedSecrets：空串回填磁盘现值，非空新值保留', () => {
    const masked = maskSensitiveFields(validConfig())
    masked.jwt.secret = 'brand-new'
    const filled = fillMaskedSecrets(masked, validConfig())
    expect(filled.jwt.secret).toBe('brand-new')
    expect(filled.smtp.password).toBe('p')
    expect(filled.admin.password).toBe('p')
    expect(filled.admin.jwt_secret).toBe('j')
  })

  it('磁盘上本就没有该敏感值时，掩码空串原样保留（不虚构默认值）', () => {
    const onDisk = validConfig()
    onDisk.smtp.password = null as unknown as string
    const filled = fillMaskedSecrets(maskSensitiveFields(onDisk), onDisk)
    expect(filled.smtp.password).toBe('')
    expect(filled.jwt.secret).toBe('my-jwt-secret')
  })
})
