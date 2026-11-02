# Poem Launcher

Poem Launcher 是一个 Windows x64 的《崩坏：星穹铁道》启动器，支持私服、官服、B服三种登录渠道，并提供战斗时停、帧率上限、解除多开、移动端 UI、角色去虚化、游戏语言切换和本地 Lua 脚本等功能。

桌面壳使用 [Wails v3](https://v3.wails.io/)，后端为 Go，界面为 React + TypeScript + Vite，原生桥接模块为 C。

## 功能

| 分类 | 内容 |
| --- | --- |
| 登录渠道 | 私服 / 官服 / B服。官服和 B服共用同一个正式国服客户端目录，启动时切换 `config.ini` 的渠道字段；B服首次使用需在启动栏下载官方渠道组件（约 85 MB）。 |
| 游戏功能 | 战斗时停（按住快捷键暂停、松开恢复）、帧率上限、解除多开、移动端 UI、角色去虚化。下次启动游戏时生效。 |
| 网络代理 | 仅私服。把游戏流量转发到本地服务器、预设服务器或自定义地址；可选 RSA 公钥替换和游戏内网页重定向。 |
| 游戏语言 | 分别设置文字和语音语言；首次修改前自动备份，可一键恢复。 |
| Lua 脚本 | 添加本地 UTF-8 `.lua` 文件，游戏进入可游玩场景后按顺序自动执行，也可对运行中的游戏手动重新执行。最多 16 个脚本、合计 4 MiB，脚本不经过服务器。 |

## 使用

1. 运行 `Poem.exe`，确认管理员权限提示。
2. 点击启动栏左侧的文件夹按钮，选择游戏目录中的 `StarRail.exe`。
3. 在启动栏选择登录渠道；在顶部导航打开各分类调整设置，修改会自动保存。
4. 点击「启动游戏」。开启解除多开后，可继续点击「再开一个」。

有游戏运行时，关闭按钮或 Alt+F4 只会最小化 Poem，以保持代理连接；游戏全部退出后才会真正关闭。

### 本地文件

- 配置、代理证书、渠道备份、语言备份等位于 `Poem.exe` 同目录的 `Poem-data/`。
- 每次启动的运行时模块释放在系统临时目录 `Poem-runtime-*`，游戏退出后清理。
- 首次启用代理时，会生成 Poem Local Proxy 根证书并安装到当前用户的受信任根证书存储。
- 启动时临时合并 `StarRail_Data/config.ini` 中的功能配置，模块加载后恢复原文件的完整字节。

## 项目结构

```
.
├── main.go                 入口：嵌入前端与模块，创建窗口并注册服务
├── build.ps1               一键构建脚本
├── assets/                 嵌入 EXE 的模块与图标（DLL 不入库，见 assets/README.md）
├── build/windows/          图标、清单、版本信息
├── frontend/               界面（React + Vite）
│   └── src/
│       ├── App.tsx         主界面：导航、设置面板、启动栏
│       ├── components.tsx  通用组件：开关、卡片、滑动选择条
│       ├── constants.ts    版本号、服务器预设、按键名等常量
│       ├── api.ts          与 Go 服务通信的封装
│       └── styles.css
├── internal/
│   ├── launcher/           核心服务：设置、启动流程、渠道切换、Lua、去虚化、崩溃记录
│   ├── injector/           进程创建与 DLL 注入
│   ├── netproxy/           私服 HTTP 代理、证书、RSA 替换、网页重定向
│   ├── gamelang/           游戏语言读取与修改（assetmeta / excel 为数据格式解析）
│   ├── gamever/            游戏版本文件解析
│   ├── dialog/             文件选择与打开目录
│   └── window/             窗口最小化与关闭
├── native/
│   ├── lua-entry/          本地 Lua 桥接（C + MinHook）
│   ├── network-entry/      原生网络桥接（C）
│   └── trampoline-fixture/ 测试用固件
├── scripts/                原生模块构建、图标生成
└── tools/                  资源生成（winres）与测试辅助程序
```

## 构建

需要：

- Windows x64，WebView2 Runtime（系统通常自带）
- Go 1.26.1+
- Node.js 22+ 与 pnpm
- Zig 0.13（仅在重新编译原生模块时需要）

步骤：

1. 准备 `assets/` 下的模块，清单见 [assets/README.md](assets/README.md)。`NetworkEntry.dll` 和 `LuaEntry.dll` 可以从源码编译：

   ```powershell
   .\scripts\build-network.ps1
   .\scripts\build-lua.ps1
   ```

   Zig 不在 PATH 中时，可通过环境变量 `ZIG` 或 `-Zig` 参数指定路径。

2. 在仓库根目录运行：

   ```powershell
   .\build.ps1
   ```

   脚本依次完成前端构建、Windows 资源生成、`go test ./internal/...` 和 Go 编译，产物为 `dist/Poem.exe` 及带版本号的副本。

开发界面时可以单独运行前端，在浏览器中以预览模式打开（不连接 Go 服务）：

```powershell
cd frontend
pnpm install
pnpm dev
# 打开 http://localhost:5173/?preview
```

### 版本号

- 界面：`frontend/src/constants.ts` 中的 `VERSION`
- EXE 版本信息：`build/windows/info.json`

## 测试

```powershell
go test ./internal/...
```

部分测试需要额外的固件或真实游戏文件，未配置时会自动跳过。相关环境变量见各测试文件，以及 `native/lua-entry/README.md`、`native/network-entry/README.md`。

## 说明

- `ProxyHook.dll`、`GameFeatures.dll`、`VisualPatch.dll` 为预编译模块，不在本仓库中分发，也不是从本仓库源码编译的。
- 游戏版本更新后，模块的特征码可能失效，需要更新模块。
- 时停、帧率、多开、移动端 UI 等功能依赖游戏内实测，自动化测试只覆盖启动器本身的逻辑。
## 致谢与来源

Poem Launcher 的部分代码来自 [Cyrene Launcher](https://github.com/Mar7thDev/CyreneLauncher)（Mar7thDev），其上游为 Firefly Shelter 的 Firefly Launcher。两者均以 MIT License 发布，本仓库保留其原始版权声明（见 [LICENSE](LICENSE)）。

| 本仓库 | 来源 |
| --- | --- |
| `internal/netproxy` | Cyrene Launcher `pkg/patch-proxy`（私服代理、证书、RSA 与 ec2b 处理） |
| `internal/injector` | Cyrene Launcher `pkg/injector` |
| `internal/gamelang`、`internal/gamelang/assetmeta`、`internal/gamelang/excel` | Cyrene Launcher `internal/language-service`、`pkg/language-patch` |
| `internal/gamever` | Cyrene Launcher `pkg/models` 中的版本文件解析 |
| `internal/dialog`、`internal/window` | 沿用 Cyrene Launcher 的服务结构（`fs-service`、`app-service`） |

代理钩子 `ProxyHook.dll` 是配合上述代理使用的预编译模块，启动器向它传递参数的环境变量沿用了 `CYRENE_*` 命名。以上代码在 Poem 中经过了修改和扩展，问题请反馈到本仓库，不要打扰上游项目。

其他参考：

- 官服/B服渠道切换参考了 [Collapse Launcher#350](https://github.com/CollapseLauncher/Collapse/issues/350) 与 [Starward](https://github.com/Scighost/Starward) 的 HoYoPlay 接口实现；渠道组件来自 `hyp-api.mihoyo.com` 的 `getGameChannelSDKs` 接口。
- `native/lua-entry/minhook` 为 [MinHook](https://github.com/TsudaKageyu/minhook) v1.3.4，使用其自带许可证。

## 许可

本项目以 [MIT License](LICENSE) 发布，并保留上游 Firefly Shelter 的版权声明。游戏美术资源版权归原权利人所有。
