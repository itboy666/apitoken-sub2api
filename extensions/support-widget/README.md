# APIToken 客服浮窗插件

这是独立的静态网站 `.s2plugin`：实际访客浮窗的 HTML、CSS、交互脚本，以及管理员配置页面都在此包内。宿主仅负责校验、生命周期、隔离加载与有限的消息桥接，不包含客服 UI。

通过 Sub2API **插件管理**安装、配置、测试、启用、停用和卸载。没有单独的内置客服开关。停用后新页面不再加载包；已打开的页面会在宿主下一次状态刷新（最长约 15 秒）移除浮窗。卸载后不会回退到旧内置配置。

## 包结构

- `manifest.template.json`：插件身份与宿主版本要求。
- `ui/`：独立配置页面，通过现有 UI Bridge 读取和保存本插件配置。
- `public/`：实际访客界面，支持桌面/手机、中文/英文、深色和减少动态效果。
- `build.py`：计算所有资源哈希、签名并打包。
- `__tests__/`：在 DOM 环境中运行包内脚本的交互与边界测试。

配置包括面板标题、欢迎语、客服名称/联系方式/链接、QQ 群号/邀请链接和网站工单入口。安装包不内置任何实际客服号码。宿主仅公开这组经校验字段；插件没有账号范围、服务器进程或网络访问权限。

当前包版本为 `1.0.4`，声明支持已验证的定制宿主 `2.23.0-apitoken.6`。原版 v2.23.0 尚无网站插件能力，不能直接安装这个静态包。今后修改客服 UI 只需升级本插件包；更换宿主版本时先验证兼容性并更新清单。

如果配置读取失败，可先重试。明确确认后才能进入恢复模式；恢复表单的联系方式、群号和链接均留空，仅使用通用标题等默认选项，由管理员手动填写。只有保存才会替换插件的整份旧配置；旧值不会自动恢复，必要时先核对备份。未确认前表单不会开放，也不能保存默认值。

## 构建与签名

从仓库根目录执行（发布私钥必须在仓库外）：

```sh
python3 extensions/support-widget/build.py \
  --key /absolute/private/path/publisher-ed25519.pem \
  --openssl /path/to/openssl-3 \
  --output /absolute/output/apitoken-support-widget-1.0.4.s2plugin
```

使用 Ed25519 对 `manifest.json` 精确原始字节签名。宿主为 `local.apitoken.support-widget` 单独固定 `apitoken-support-widget-v1` 公钥；私钥不进入包、仓库或服务器。不要为安装本插件打开 `allow_unsigned`。

仅本地包结构测试可用 `--unsigned` 替代 `--key`；这类包不用于生产。

## 消息边界

后台 UI 使用 `sub2api-plugin-ui` / `sub2api-plugin-host` 协议。普通修改通过 `config.save`；管理员明确确认后的恢复覆盖使用 `config.recover`，宿主须支持该恢复请求并返回 `config.recover.result`。二者均携带 `config` 对象，成功结果返回 `ok: true` 和规范化后的 `config`；两种写请求的超时均为 120 秒。恢复成功后的普通修改重新使用 `config.save`。访客 UI 使用 `sub2api-public-plugin` / `sub2api-public-host` 协议，双方核对窗口来源、随机 `bridge_token` 与请求 ID。

访客 UI 接收 `widget.init`：公开配置、语言、主题、宿主视口宽高和网站工单功能状态。可请求 `widget.resize`、复制已公开联系方式、打开已配置的安全链接。主程序不会把用户令牌、Cookie、账号或内部配置传入 iframe。iframe 不获得 `allow-same-origin`。

## 测试

已安装前端依赖后，从仓库根目录使用 Node 执行：

```sh
node --test extensions/support-widget/__tests__/package.test.mjs
```

测试只使用本地 DOM；不访问服务器或发送客服消息。
