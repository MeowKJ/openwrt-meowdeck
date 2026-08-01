# MeowDeck

MeowDeck 是一个运行在 OpenWrt 上的轻量本地服务导航与心跳面板。前端由
React 构建，后端是单个 Go 进程；不需要数据库，心跳历史只保存在内存中，
适合直接放在家用路由器上。

默认安装只显示两张不可删除的卡片：

- GL.iNet 管理后台：`http://meow.lan/router`
- LuCI 高级管理：`http://meow.lan/luci`

OpenClash、Tailscale、QQ 农场、Home Assistant 或其他服务都由用户在页面中
自行添加。

## 功能

- 在页面中添加、删除服务卡片，配置会原子写入 `/etc/meowdeck/config.json`
- HTTP、TCP、Ping 和本机进程四种心跳检查
- 默认入口 `http://meow.lan/项目标识`
- 可选入口 `http://自定义.meow.lan`
- 可选子域名反向代理；关闭代理时入口使用 `307` 跳转到实际后台地址
- 内存心跳历史，不持续写路由器闪存
- 自适应桌面与手机屏幕，支持系统“减少动态效果”设置
- ARM64、AMD64、ARMv7、MIPS 和 MIPSLE 发布包
- Stable/Edge 自动发布与失败回滚更新

## 地址规则

添加一个项目标识为 `home-assistant` 的服务后，它始终拥有默认入口：

```text
http://meow.lan/home-assistant
```

如果同时填写子域名 `ha`，主入口会显示为：

```text
http://ha.meow.lan
```

安装程序会让 dnsmasq 同时解析 `meow.lan` 和 `*.meow.lan`，所以新增子域名时
不需要再次修改 DNS。启用“保持自定义域名”后，Go 后端会反向代理实际地址；
某些使用绝对路径、WebSocket 或严格 Cookie 域名的后台可能不兼容，此时关闭
代理并使用跳转模式即可。

## OpenWrt 安装

下载与路由器架构匹配的发布包，解压后以 root 运行：

```sh
./install.sh .
```

安装器会自动读取 `network.lan.ipaddr`，然后完成：

- 安装 `/usr/bin/meowdeck` 与 procd 开机服务
- 首次生成 `/etc/meowdeck/config.json`
- 添加 `/etc/nginx/conf.d/meowdeck.conf`
- 配置 `meow.lan` 和 `*.meow.lan` 的 dnsmasq 解析
- 检查 nginx 配置和 MeowDeck `/healthz`
- 安装失败时恢复原有文件、DHCP 配置和服务状态

完成后，在使用该路由器 DNS 的设备上打开 <http://meow.lan>。GL.iNet 原管理页
和 LuCI 的 IP 地址入口不会被替换。

## 页面与 API 添加服务

页面右上角的“添加服务”支持名称、项目标识、说明、分类、图标、实际 URL、
心跳方式、检查目标、可选子域名和反向代理。

也可以调用同源 API：

```sh
curl -X POST http://meow.lan/api/v1/services \
  -H 'Content-Type: application/json' \
  -H 'X-MeowDeck-Edit: 1' \
  -d '{
    "id": "home-assistant",
    "slug": "home-assistant",
    "subdomain": "ha",
    "name": "Home Assistant",
    "description": "全屋智能家居控制中心",
    "category": "smart-home",
    "icon": "house",
    "url": "http://192.168.8.178:8123",
    "proxy": false,
    "probe": {
      "type": "http",
      "target": "http://192.168.8.178:8123"
    }
  }'
```

删除自建卡片：

```sh
curl -X DELETE http://meow.lan/api/v1/services/home-assistant \
  -H 'X-MeowDeck-Edit: 1'
```

`X-MeowDeck-Edit` 用于阻止普通跨站表单直接修改配置，不是身份认证。MeowDeck
默认只监听 `127.0.0.1:9080` 并由 OpenWrt nginx 暴露，请只在可信局域网使用，
不要直接映射到公网。

## 本地开发

需要 Node.js 22+、npm 和 Go 1.23+：

```sh
make check
make test
make build
./bin/meowdeck -config configs/config.example.json -listen 127.0.0.1:9080
```

打开 <http://127.0.0.1:9080>。发布前可生成 ARM64 安装包：

```sh
make package VERSION=dev
```

## 更新通道

- `stable`：`v1.0.0` 这类正式 GitHub Release
- `edge`：`main` 分支每次通过 CI 后产生的预览版本

修改 `/etc/meowdeck/update.conf` 中的 `CHANNEL`，然后执行 `meowdeck-update`。
更新程序会校验 SHA-256；若 `/healthz` 未恢复，会自动换回旧二进制。

## License

MIT
