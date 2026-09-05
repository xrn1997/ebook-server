import { describe, it, expect } from 'vitest'
import { parseEnv, serializeEnv, mergeConfigToYaml, splitConfigToEnv, validateConfig } from '../config'
import type { AppConfig } from '../config'

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
})
