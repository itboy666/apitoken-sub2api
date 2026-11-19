# 网站客服静态插件

`website.support_widget.v1` 是网站客服浮窗专用的静态能力。完整管理界面和访客界面都在 `.s2plugin` 包内。安装、配置、测试、启用、停用、升级和卸载复用 `/admin/plugins`，不需要独立系统设置页，也不写入核心 Vue 组件。

## 清单与文件

```json
{
  "schema_version": 1,
  "id": "local.apitoken.support-widget",
  "name": "APIToken 客服",
  "version": "1.0.3",
  "requires": {
    "sub2api": "=2.23.0-apitoken.5",
    "recommended_sub2api_version": "2.23.0-apitoken.5",
    "tested_sub2api_versions": ["2.23.0-apitoken.5"],
    "plugin_protocol": 1,
    "transport_api": 1,
    "ui_bridge": 1
  },
  "capabilities": [
    {"id": "website.support_widget.v1", "platform": "website", "account_type": "none"}
  ],
  "ui": {"entrypoint": "ui/index.html"},
  "public_ui": {"entrypoint": "public/index.html"},
  "files": {
    "ui/index.html": "<实际文件的 SHA-256>",
    "public/index.html": "<实际文件的 SHA-256>"
  }
}
```

示例中的哈希占位符必须在打包时替换。`runtimes` 省略或为空对象；禁止混合能力、二进制、任意私有目录或路径跳转。允许的资源位于 `ui/` 或 `public/` 下，扩展名为小写 `.html`、`.js`、`.css`、`.json`、`.svg`、`.png`、`.jpg`、`.jpeg`、`.webp`、`.ico`、`.woff`、`.woff2`。公开入口必须是声明过的 `public/*.html` 文件。

生产包必须签名。`local.apitoken.support-widget` 使用宿主固定的 `apitoken-support-widget-v1` 公钥，其他插件使用已审核的 `trusted_publishers`。不应为安装客服插件启用 `allow_unsigned`。

宿主以规范化完整清单的 SHA-256 作为静态包身份，复用现有 `binary_sha256` 存储字段。任何资源、版本或元数据变化都会改变身份；前端应使用宿主返回值，不自行计算。安装和跨实例恢复均验签并核对所有文件。

## 配置与生命周期

管理 UI 复用 [UI Bridge](ui-bridge.md)。`config.save` 由宿主验证以下字段并加密保存：`title`、`description`、`service_label`、`service_contact`、`service_url`、`qq_group_number`、`qq_group_url`、`use_ticket`。`enabled` 是公开配置的派生状态，不能代替插件启停；启用只由原生生命周期 API 控制。其他字段被拒绝，配置上限为 16 KiB。

普通保存拒绝覆盖无法读取的配置。管理员需先停用插件，在配置 UI 中明确确认重新填写，然后使用 `config.recover` 消息；宿主经既有二次验证调用 `POST /admin/plugins/:id/config/recover`，并带 `X-Plugin-Config-Recovery: confirmed`。服务端仅允许已停用、旧配置无法读取的网站客服插件恢复，并再次验证配置及资源。响应消息类型为 `config.recover.result`，与普通保存一样返回规范化配置。可正常读取的配置应使用普通保存。

插件配置使用独立的 `${DATA_DIR}/secrets/plugin-config.key`（默认数据目录下同路径），权限为文件 `0600`、目录 `0700`。新密文标记为 `plugin-config-v1:`。原始无标记密文只在宿主显式配置了原固定加密密钥时读取；它们不会在读取时被覆盖。应将插件密钥与数据库一起备份；短期 UI 会话仍使用原来的会话加密方式，重启后重新打开配置即可。

已有新格式配置但插件密钥缺失时，普通保存不会生成替代密钥；应优先恢复原密钥。明确恢复只允许当前配置是该密钥唯一使用者（或尚无新格式配置）时创建缺失的插件密钥，绝不替换已有密钥。其他插件仍依赖缺失密钥时必须恢复备份，不能逐个清空重建。用户二次验证及账号凭据密钥保持原用途。

`config.test` 校验兼容性、资源完整性和联系方式，不启动进程、不连接上游。状态接口返回静态能力是否已启用。上传、保存、测试和启停仍使用管理员认证和既有二次验证保护。新包默认停用；同 ID 更新前必须停用；同 ID 更新保留加密配置；卸载前必须停用。

静态插件不会创建 gRPC 进程、访问账号目录、占用出站路由或改变现有 OpenAI Transport。现有启用能力唯一约束保证同一网站客服能力只启用一个插件。旧 `support_widget_config` 设置行保留以便显式迁移，公开设置不再读取它，旧专用设置路由已移除。

## 访客资源

`GET /api/v1/plugins/public` 返回标准成功封装，`data` 为数组，每项仅包含 `id`、`plugin_key`、`name`、`version`、`entrypoint_url` 和公开联系方式 `config`。停用、不兼容、包损坏或配置无效的插件不出现在数组中。

入口形式为 `/api/v1/plugins/public/:id/assets/:hash/public/index.html`，其中 `hash` 是宿主返回的包身份，尾部保留清单中的完整 `public/` 资源路径。资源端点仅允许清单声明的 `public/` 文件，每次请求复验内容哈希、当前启用状态与包身份。`ui/`、清单、签名、配置密文和包原件无法通过该接口获取。

列表和资源均设置 `Cache-Control: no-store`。公开资源设置 `sandbox allow-scripts` CSP，不授予同源权限、弹窗、表单或网络访问。通用访客宿主从公开接口读取状态，按周期和重新聚焦刷新；停用或卸载后移除 iframe。管理 UI 继续使用带时限的能力 URL，访客 iframe 不获得管理 Token 或管理 Bridge 权限。
