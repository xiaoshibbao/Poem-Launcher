# assets

构建时由 `main.go` 通过 `go:embed` 打包进 Poem.exe。仓库只保存图标源文件，DLL 需要在构建前放进本目录。

| 文件 | 作用 | 来源 |
| --- | --- | --- |
| `ProxyHook.dll` | 私服流量代理钩子 | 预编译模块，不随仓库分发 |
| `GameFeatures.dll` | 战斗时停、帧率上限、解除多开、移动端 UI | 预编译模块，不随仓库分发 |
| `VisualPatch.dll` | 角色去虚化 | 预编译模块，不随仓库分发 |
| `NetworkEntry.dll` | 原生网络桥接 | `scripts/build-network.ps1` 由 `native/network-entry` 编译 |
| `LuaEntry.dll` | 本地 Lua 桥接 | `scripts/build-lua.ps1` 由 `native/lua-entry` 编译 |
| `appicon.png` | 窗口与托盘图标 | `scripts/make-icon.py` 由 `poem-icon-source.png` 生成 |

`build.ps1` 会在开始前检查这五个 DLL 是否齐全。
