# APIToken USDT · TRC20 支付插件

版本 1.0.1，兼容宿主 2.23.0-apitoken.6–.7，推荐 .7。包含独立 Go 核验进程及独立后台配置 UI；通过现有 `/admin/plugins` 安装、配置、测试、启停、卸载。

填写 TRON 收款地址和 TronScan API Key，保存并测试连接，再启用插件。API Key 在服务器加密保存且不回显。插件不需要钱包私钥。支付方式会自动出现在充值页面。

固定汇率 **1 USDT = 6.5 CNY**，沿用网站现有充值限额、手续费和到账规则。宿主 .7 起，人民币应付金额除以 6.5 后向上保留两位小数，再随机增加 0.00–0.09 USDT；有效期内同钱包金额冲突则逐次加 0.01 USDT。取消或已付的订单仍占用金额至原到期时间，过期后才可复用。尾数不增加余额；旧订单保持原精确金额。宿主 .6 沿用原六位小数规则。充值页直接打开付款弹窗，关闭后可继续同一订单；独立付款链接使用相同界面。请使用完整数量并在订单有效期内转账；TRX、假 USDT、错误金额和其他网络不自动到账。正确转账至少 20 次确认后进入原有充值账本。

正常升级保留数据库与 `data/secrets/plugin-config.key`。存在待核验订单时不能更换钱包。停用会暂停核验；如需卸载，先处理已付款订单。插件不自动退款。

构建：从 `backend/` 编译 `./cmd/usdt-trc20-plugin` 到本目录 `bin/linux-amd64/usdt-trc20`（Linux amd64）及 `bin/darwin-arm64/usdt-trc20`（本地验证）。使用 `build.py --output <path>.s2plugin --key <outside-repo-private.pem> --openssl <OpenSSL3>` 签名。`bin/` 为构建产物，不提交源码；私钥和 API Key 不打包。

完整协议、金额规则和官方接口来源见 [支付插件文档](../../backend/pkg/pluginapi/docs/usdt-payments.md)。
