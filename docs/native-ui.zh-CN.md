# 用 MyGo 原生 UI 重写：和网页版的区别与代码导读

[stock-float-mygo](https://github.com/lesenelir/stock-float-mygo) 用 MyGo 的网页窗口：Go 负责数据，面板和设置窗口是 Vite 打包的网页，跑在 WKWebView 里。这个项目用 MyGo 的**原生 UI**（`ui` 包）把这两个窗口重写了一遍：没有 HTML、没有 JavaScript、没有 WebView，界面也是 Go 代码，由 MyGo 用 Metal 在 GPU 上绘制。

## 一、哪些代码原样复用

和界面无关的部分直接从网页版复制过来，只改了 import 路径：

| 目录/文件 | 内容 |
|---|---|
| `internal/market` | 代码解析、行情模型、30 分钟过期 |
| `internal/config` | `~/.config/stock-float/config.json`，三个版本共用 |
| `internal/feed` | 腾讯、Finnhub、长桥三个数据源 |
| `internal/l10n` | 中英文文案 |
| `store.go` | 配置 + 行情 + 断线状态 |
| `menus.go` | 右键菜单、菜单栏图标菜单、Dock 菜单、主菜单（都是 `mygo.Menu`，两种窗口通用） |
| `hotkey.go`、`updates.go`、`native_*.go` | 快捷键、更新、macOS 面板行为 |

这正好说明把业务和界面分开的价值：换一套 UI 技术，数据层一行不用改。

## 二、界面层怎么变

### 1. 数据流：从"推送事件"变成"每帧读取"

网页版：

```
Store 变化 → Go 计算 PanelState → PanelChanged.Emit(panel, state) → 页面 events.panel.on(render)
```

原生版：

```
Store 变化 → panelWindow.Invalidate()（请求一帧）→ MyGo 在主线程调用 panelView(c)
          → 视图里 a.state() 读 Store 快照，直接构建界面
```

原生 UI 是**即时模式**：视图是"状态 → 界面"的函数，MyGo 只在需要时调用它（有输入、`Invalidate`、`After`、动画），空闲时一帧也不画。所以不需要 `mygo.Bind`、`mygo.NewEvent`、`src/mygo.ts`，也没有 Vite、Bun 和 `package.json`。

### 2. 对照表

| 网页版 | 原生版 |
|---|---|
| `index.html` + `src/panel.ts` + `src/panel.css` | `panel_view.go` 的 `panelView` |
| `settings.html` + `src/settings.ts` + `src/settings.css` | `settings.go` 的 `settingsForm.view` |
| `services.go`（`Panel`、`Settings` 两个绑定服务） | 不需要：视图直接调用 Go |
| `PanelChanged` 事件 | `Window.Invalidate()` |
| CSS 变量 + `prefers-color-scheme` | `c.Theme()`，自动跟随系统深浅色 |
| `font-variant-numeric: tabular-nums` | `.FontFeatures("tnum")` |
| 按 symbol 复用 DOM 节点，用 WAAPI 做闪烁 | `c.Key(symbol)` 保持行的身份，用 `Keyframes` 做闪烁 |
| `setInterval` 每分钟检查是否过期 | `c.After(下一行过期还剩多久)`，到点才画一帧 |
| `ResizeObserver` → `Panel.resize` | 读上一帧的 `content.Bounds()` → `fitPanel` |
| CSS mask 给 PNG 图标上色 | `Painter.ImageRegion(..., ImageOptions{Tint})` |
| `isCallError(err)` 显示"无法识别" | `ui.Field(...).Error(f.invalid)`，输入框自动变红框 |
| 页面里 `--app-region: drag` / `contextmenu`（只用于 Linux 和 Windows） | macOS 上仍由 `native_darwin.go` 处理拖动和右键 |
| `mygo.config.ts` | `mygo.json`（没有 JS，用 JSON 配置） |
| `bun run dev` / `bun run build` | `go tool mygo dev` / `go tool mygo build` |

### 3. 测试：视图也能单测

`views_test.go` 用 `ui.NewTester` 在没有窗口的情况下运行视图：它渲染帧、按文字找元素、模拟点击和输入。例如"设置里填了无法识别的代码，点保存后显示错误、且不保存"：

```go
f := &settingsForm{app: testApp(t, nil), s: l10n.For(config.LanguageEn, ""), symbols: "AAPL\n苹果\n1234"}
tt := ui.NewTester(f.view, 392, 480)
tt.Click("Save")
tt.HasText("Not recognized: 苹果, 1234") // true
```

网页版的页面没有这样的测试；原生版的界面和业务一样，`go test` 就能覆盖。

## 三、写的时候踩过的坑

1. **隐藏的原生 UI 窗口不绘制帧。** 网页版是"窗口先隐藏，页面量好尺寸再显示"。原生版照搬后，面板永远没出现：视图拿不到上一帧的 `Bounds()`，也就永远不会去显示窗口。现在的做法是：按行数估一个尺寸，立刻显示；第一帧量到真实尺寸后再调整，左上角不动。设置窗口同理。
2. **`mygo.App.Locale()` 会跳到主线程等待。** 在测试里没有运行中的 App，`Tester` 卡死在这里。改为启动时在 `main` 里读一次，存进 `App.locale`。
3. **在视图里改窗口尺寸要推迟。** 视图在构建帧的时候运行，不能在里面同步 `SetBounds`。所以用 `go mygo.RunOnMain(...)`，等这一帧结束后再改。
4. **元素只在当前这一帧有效。** `ui.Element` 不能存进 App 的状态里（`go tool mygo vet .` 会检查）。要跨帧保留的东西（行情、上次价格、闪烁计数）放在 `App` 的字段里；行的身份靠 `c.Key(symbol)` 维持。

## 四、体积与内存（实测）

| | 网页版 | 原生版 |
|---|---|---|
| App 体积（arm64） | 10.8 MB | 13.2 MB |
| DMG | 3.4 MB | 4.0 MB |
| 主进程 phys_footprint（开发版） | 38 MB | 41 MB |
| 另外的进程 | WKWebView 的 WebContent、GPU、Networking 辅助进程 | 无 |

原生版的二进制更大，因为 Go 写的渲染器和组件库都编译进了程序。网页版主进程差不多，但每个网页还要额外跑 WebKit 的辅助进程。两者都远大于 Swift 原版（1.6 MB、约 17 MB 内存）。

## 五、建议的阅读顺序

1. `main.go` → `app.go`：入口，和网页版比少了 `Bind`。
2. `panel_view.go`：原生 UI 的核心。先看 `panelView` 的整体结构，再看 `quoteRow` 里的布局（`Row`/`Column`/`Gap`/`Grow`）、颜色、`Keyframes` 闪烁和 `Draw` 画图标。
3. `settings.go`：表单组件（`Field`、`TextArea`、`Checkbox`、`TextInput`、`Button`）和快捷键（`c.Shortcut`）。
4. `views_test.go`：`ui.Tester` 的用法。
5. `panel.go`、`native_darwin.go`：窗口层面的东西，和网页版几乎一样。

MyGo 原生 UI 的文档入口：<https://mygo.egoist.dev/docs/ui>，其中「Views」「Layout」「Input」「Motion」「Testing」五篇和这个项目最相关。
