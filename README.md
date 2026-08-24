# MeowDeck Auto-NAV

MeowDeck 是运行在 OpenWrt 上的只读局域网导航服务。它从明确允许的 LAN
接口读取 DHCP 和邻居信息，自动发现 IPv4 客户端与少量常用 Web 服务，并以
低频闪存写入保存确认后的故障历史。

路由器只运行一个嵌入前端资源的 Go 二进制，不需要 Node.js、Go、数据库或
Docker。编译、测试、签名和发布全部由 GitHub Actions 完成，路由器通过
Pull 更新器自动安装新版本。

## V1 行为

- 只发现 lanInterfaces 中列出的 OpenWrt 逻辑接口；默认只有 lan
- V1 只使用 IPv4，不将“私有地址”本身视为扫描授权
- 客户端每 60 秒发现一次，新设备立即执行服务探测
- 已在线设备每 10 分钟重新探测常用端口
- HTTP 探测最多读取 4 KiB，禁止自动重定向
- 设备连续出现两轮后建立，连续缺失三轮后离线
- 服务连续发现两轮后建立，候选服务消失不产生故障
- 稳定服务连续失败三次才记录故障，连续成功两次才恢复
- 设备离线时记录一个设备事件，并抑制其子服务重复故障
- 原始探测与发现缓存只在 RAM；闪存只保存故障边界和批量摘要

完整冻结边界见 [V1 架构](docs/architecture-v1.md)。

## 配置

/etc/meowdeck/config.json 只保留运行入口和 LAN 白名单：

    {
      "listen": "127.0.0.1:9080",
      "hostname": "meow.lan",
      "lanInterfaces": ["lan"]
    }

旧配置中的管理令牌和服务列表会被安全忽略，不需要人工迁移。

## 只读 API

    GET /healthz
    GET /api/v1/version
    GET /api/v1/catalog
    GET /api/v1/devices/<id>
    GET /api/v1/incidents

MeowDeck 不提供 POST、PUT 或 DELETE 管理接口。

## 本地验证

需要 Go 1.23 和 Node.js 22：

    make check
    make test
    make build
    ./bin/meowdeck self-test -config configs/config.example.json

浏览器实际运行 CI 编译后的 JavaScript、HTML 和 CSS；OpenWrt 上不安装 npm
或 TypeScript。

## OpenWrt 安装

从 GitHub Release 下载与架构匹配的归档，首次安装执行一次：

    ./install.sh .

安装器配置 procd、nginx、dnsmasq 和自动更新服务。之后无需 SSH、手动添加
服务或手动升级。要从刷机开始完全零操作，需要把安装包集成进固件。

## 自动更新

业务二进制同时提供：

    meowdeck serve
    meowdeck update
    meowdeck self-test
    meowdeck version

meowdeck-update 只是调用 meowdeck update 的薄壳。Go 更新器完成：

- Ed25519 manifest 验签和 key ID 轮换
- SHA-256 与文件大小检查
- channel 和 sequence 防降级
- 候选二进制 self-test
- /usr/bin 同文件系统原子替换
- 精确版本健康检查
- Last Known Good 自动回滚

Edge 每 15 分钟检查，Stable 每 6 小时检查，并加入随机抖动。稳定通道通过
GitHub latest Release 获取签名 manifest。

发布前需要配置：

    Secret:   MEOWDECK_SIGNING_PRIVATE_KEY
    Variable: MEOWDECK_KEY_ID
    Variable: MEOWDECK_TRUSTED_KEYS_B64

MEOWDECK_TRUSTED_KEYS_B64 是 base64 编码的 JSON，JSON 中 key ID 映射到
base64 Ed25519 公钥。manifest 格式见
[manifest schema](docs/manifest.schema.json)。

## License

MIT
