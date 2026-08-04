# 钉钉 OAuth 登录与离职自动禁用

本文档记录企业内部应用钉钉 OAuth 登录，以及绑定员工离职后自动禁用账号与 API Token 的机制。两者配套：登录负责"谁能进来"，离职检测负责"人走后能自动关门"。

## 功能总览

| 能力 | 触发时机 | 行为 |
| --- | --- | --- |
| 钉钉 OAuth 登录 | 用户点击登录 / 绑定 | 企业内应用授权码流程，按 unionId 识别用户 |
| 自动用户名 | 首次钉钉登录注册 | `dingtalk_<递增数字>`，不暴露 unionId |
| 登录时在职校验 | 每次钉钉登录 / 绑定 | 确认已离职 → 拒绝登录 |
| 每日离职巡检 | 每天凌晨 3 点后首次触发 | 确认已离职 → 禁用账号 + 全部 API Token + 吊销会话 |

## 钉钉 OAuth 登录

企业内应用授权码流程。后端持 `AppKey`/`AppSecret`/`CorpId`，`AppSecret` 仅服务端使用，不暴露给前端；`/api/status` 只下发 `dingtalk_app_key` 与 `dingtalk_corp_id`。

- 外部浏览器授权 URL 需要应用级 access token，由后端 `DingTalkAuthUrl` 组装：`oauth2/accessToken` 换取应用 token 后拼接 `login.dingtalk.com/oauth2/auth`。
- 用户身份以 `unionId` 为准（`GetUserInfo` 返回 `ProviderUserID = unionId`），写入 `users.dingtalk_id`。
- 应用必须是钉钉开放平台"企业内部应用"，并在"登录与分享"中配置回调域名；localhost 无法被钉钉回调，本地测试需真实域名或内网穿透。

### 自动用户名

首次登录注册的新用户，用户名强制为 `dingtalk_` 前缀 + 递增数字（`dingtalk_1`、`dingtalk_2` …），在 `findOrCreateOAuthUser` 中按 provider 类型覆盖。unionId 是钉钉内部标识、可读性差，不进入 username；显示名仍取钉钉昵称。

## 离职检测：在职状态判定

`CheckUserActive(ctx, unionId) (bool, error)` 调用钉钉 `topapi/user/getbyunionid`，以 errcode 判定：

| errcode | 含义 | 判定 |
| --- | --- | --- |
| `0` | 在职 | `active=true` |
| `60121` | 未找到对应员工（已离职/不在通讯录） | `active=false`，**触发禁用** |
| 其他（权限缺失 `60011`、限流、网络错误等） | 状态未知 | 返回 error，**绝不触发禁用** |

这是核心安全不变量：只有 `60121` 这一"明确不在组织内"的信号才禁用。权限没开通、网络抖动、钉钉侧异常都判"状态未知"并跳过——宁可漏判，不能误伤在职员工。

应用 access token 会被缓存复用：Redis 可用走 `RedisGet/RedisSet`（key `dingtalk:app_access_token`，TTL = expireIn − 5 分钟），否则进程内 `sync.Mutex` 内存缓存。登录、在职校验、巡检共用，避免每次重新取 token。

## 登录时在职校验（兜底）

`GetUserInfo` 拿到 unionId 后先调 `CheckUserActive`：

- 确认离职 → 返回 `oauth.user_left_organization` 错误，前端提示"该钉钉账号已不在企业组织内，无法登录"，登录与绑定均被拒。
- 状态未知 → 记 Warn 日志后放行，不阻断在职员工。

登录校验是兜底：员工离职到下次巡检之间若尝试登录，会被即时拦下。

## 每日离职巡检（主动回收）

`service/dingtalk_leave_check_task.go`，随后端进程启动，在 `main.go` 注册。不是外部 cron，是进程内 goroutine：

- `sync.Once` + `common.IsMasterNode` 门控：多节点部署只有主节点跑。
- 每小时 tick，`runDingTalkLeaveCheckIfDue` 比对 `atomic.Int64` 记录的 YYYYMMDD，凌晨 3 点后首次触发即执行当日巡检，当天后续 tick 跳过；`atomic.Bool` CAS 防止重叠执行。
- 巡检开关跟随配置：`dingtalk.enabled` 开启且配置了 AppKey/AppSecret 才执行，否则直接跳过，无独立设置项。

执行流程：

1. 查出所有启用、非 root（`role < 100`）、`dingtalk_id != ''` 的用户（`GetEnabledDingTalkUsers`）。
2. 逐个 `CheckUserActive`，请求间隔 200ms 防限流。
3. 确认离职 → 禁用；状态未知 → 记日志跳过，下次巡检重试。
4. 结束时 SysLog 汇总 `checked/disabled/unknown` 计数。

### 禁用动作（只禁用不删除，可逆可审计）

`disableDepartedDingTalkUser` 对确认离职的用户：

1. `user.Status = UserStatusDisabled; user.Update(false)` — 状态变更会递增 `auth_version`，使旧登录会话失效，并发布新用户缓存。
2. `DisableAllEnabledTokensByUserId` — 名下所有启用状态 API Token 批量置为禁用，并失效 Redis 令牌缓存（`InvalidateUserTokensCache`）。
3. `RevokeAllUserSessions(userId, "dingtalk_leave")` — 吊销所有存活登录会话。
4. `RecordLog(userId, LogTypeSystem, "检测到已退出钉钉企业…")` — 管理后台日志可见。

账号与 Token 均只改 `status=2`，数据保留，管理员可在后台恢复。

## 依赖的钉钉权限

离职检测（`topapi/user/getbyunionid`）要求应用在钉钉开放平台开通**通讯录成员读权限**（scope `qyapi_get_member`）。未开通时该接口返回 subcode `60011`，所有用户被判"状态未知"，巡检空跑但不误禁用。开通后每天凌晨 3 点巡检即真正生效。

申请入口（钉钉开放平台 → 应用 → 权限管理，或探测返回的直达链接）：

```
https://open-dev.dingtalk.com/appscope/apply?...#qyapi_get_member
```

## 涉及文件

| 操作 | 文件 |
| --- | --- |
| 钉钉 Provider、token 缓存、`CheckUserActive`、登录校验 | `oauth/dingtalk.go` |
| 授权 URL 组装 | `controller/oauth_dingtalk.go` |
| OAuth 回调、自动用户名、错误分发 | `controller/oauth.go` |
| 每日巡检任务 | `service/dingtalk_leave_check_task.go` |
| 任务注册 | `main.go` |
| 按在职/非 root 查钉钉用户 | `model/user.go` `GetEnabledDingTalkUsers` |
| 批量禁用 Token + 缓存失效 | `model/token.go` `DisableAllEnabledTokensByUserId` |
| 钉钉配置 | `setting/system_setting/dingtalk.go` |
| "已离职无法登录"提示 | `i18n/keys.go` + `i18n/locales/{en,zh-CN,zh-TW}.yaml` |

## 验证方式

- 完整链路已用临时测试桩验证：给 `CheckUserActive` 加"unionId 带 `TEST_LEAVE_` 前缀即判离职"的桩，造测试用户+Token，触发巡检确认账号禁用、Token 禁用、`auth_version` 递增、系统日志落库。验证后桩与测试数据已还原，未留在代码中。
- 真实环境生效前置：钉钉后台开通 `qyapi_get_member` 权限，否则巡检始终 `unknown`。
