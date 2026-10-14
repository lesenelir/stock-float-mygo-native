// Package l10n holds every user-facing string, in Chinese and English.
//
// The strings live in Go, which builds the menus, and the panel and the
// settings page receive the ones they show from Go, so the two languages are
// kept in one place.
package l10n

import (
	"fmt"
	"strings"

	"stock-float-mygo-native/internal/config"
	"stock-float-mygo-native/internal/feed"
	"stock-float-mygo-native/internal/market"
)

// Strings says the interface in one language.
type Strings struct{ zh bool }

// For resolves the user's language setting; config.LanguageSystem follows
// preferred, the system's first preferred language, such as "zh-Hans-CN".
func For(setting config.Language, preferred string) Strings {
	switch setting {
	case config.LanguageZh:
		return Strings{zh: true}
	case config.LanguageEn:
		return Strings{}
	}
	return Strings{zh: strings.HasPrefix(preferred, "zh")}
}

// Chinese reports whether the strings are Chinese.
func (s Strings) Chinese() bool { return s.zh }

func (s Strings) t(zh, en string) string {
	if s.zh {
		return zh
	}
	return en
}

// Panel

func (s Strings) EmptyHint() string { return s.t("右键添加标的", "Right-click to add symbols") }

// Session is kept short: the label shares a narrow row with a price and a
// percentage.
func (s Strings) Session(session market.Session) string {
	switch session {
	case market.Pre:
		return s.t("盘前", "Pre")
	case market.Post:
		return s.t("盘后", "Post")
	}
	return s.t("夜盘", "Night")
}

func (s Strings) Outage(o feed.Outage) string {
	switch o.Kind {
	case feed.LongbridgeMissing:
		return s.t("未找到 longbridge 命令，请先安装长桥 CLI", "longbridge command not found. Install the Longbridge CLI.")
	case feed.LongbridgeLoggedOut:
		return s.t("长桥未登录，请在终端运行 longbridge auth login", "Not logged in to Longbridge. Run longbridge auth login in a terminal.")
	case feed.LongbridgeError:
		return s.t("长桥："+o.Detail, "Longbridge: "+o.Detail)
	}
	return s.t("连接中断，价格可能已过期", "Disconnected. Prices may be stale.")
}

func (s Strings) UpdateHint(version string) string {
	return s.t("新版本 "+version+" 可用，右键更新", "Version "+version+" available. Right-click to update.")
}

// Context menu

func (s Strings) UpdateTo(version string) string {
	return s.t("更新到 "+version, "Update to "+version)
}
func (s Strings) Updating() string         { return s.t("正在下载更新…", "Downloading Update…") }
func (s Strings) UpdateFailed() string     { return s.t("更新失败", "Update Failed") }
func (s Strings) OpenDownloadPage() string { return s.t("打开下载页面", "Open Download Page") }
func (s Strings) Settings() string         { return s.t("设置…", "Settings…") }
func (s Strings) HidePanel() string        { return s.t("隐藏小窗", "Hide Panel") }
func (s Strings) ShowPanel() string        { return s.t("显示小窗", "Show Panel") }
func (s Strings) DockIcon() string         { return s.t("在 Dock 中显示图标", "Show in Dock") }
func (s Strings) Appearance() string       { return s.t("外观", "Appearance") }
func (s Strings) Colors() string           { return s.t("配色", "Colors") }
func (s Strings) TextSize() string         { return s.t("字号", "Text Size") }
func (s Strings) Opacity() string          { return s.t("不透明度", "Opacity") }

func (s Strings) UpColor(c config.UpColor) string {
	switch c {
	case config.UpRed:
		return s.t("红涨绿跌", "Red up, green down")
	case config.UpMono:
		return s.t("不着色", "No color")
	}
	return s.t("绿涨红跌", "Green up, red down")
}

func (s Strings) TextSizeName(size config.TextSize) string {
	switch size {
	case config.TextMedium:
		return s.t("中", "Medium")
	case config.TextLarge:
		return s.t("大", "Large")
	}
	return s.t("小", "Small")
}

func (s Strings) ClickThrough() string {
	return s.t("鼠标穿透（按住 ⌥ 临时操作）", "Click-through (hold ⌥ to interact)")
}
func (s Strings) LaunchAtLogin() string { return s.t("开机启动", "Launch at login") }
func (s Strings) LaunchAtLoginUnavailable() string {
	return s.t("需要以打包后的 StockFloat.app 运行", "Requires running the bundled StockFloat.app")
}
func (s Strings) LanguageMenu() string { return s.t("语言", "Language") }
func (s Strings) Quit() string         { return s.t("退出", "Quit") }

// LanguageName names each language in itself, so it can be found from the
// other one.
func (s Strings) LanguageName(l config.Language) string {
	switch l {
	case config.LanguageZh:
		return "中文"
	case config.LanguageEn:
		return "English"
	}
	return s.t("跟随系统", "System")
}

// Main menu

func (s Strings) QuitApp() string     { return s.t("退出 StockFloat", "Quit StockFloat") }
func (s Strings) Edit() string        { return s.t("编辑", "Edit") }
func (s Strings) Undo() string        { return s.t("撤销", "Undo") }
func (s Strings) Redo() string        { return s.t("重做", "Redo") }
func (s Strings) Cut() string         { return s.t("剪切", "Cut") }
func (s Strings) Copy() string        { return s.t("拷贝", "Copy") }
func (s Strings) Paste() string       { return s.t("粘贴", "Paste") }
func (s Strings) SelectAll() string   { return s.t("全选", "Select All") }
func (s Strings) CloseWindow() string { return s.t("关闭窗口", "Close Window") }

// Settings

func (s Strings) SettingsTitle() string { return s.t("StockFloat 设置", "StockFloat Settings") }
func (s Strings) SymbolsLabel() string {
	return s.t("标的（每行一个）", "Symbols (one per line)")
}
func (s Strings) SymbolsHint() string {
	return s.t("美股直接写代码，如 AAPL；港股 hk00700；A 股 sh600519、sz000001",
		"US: the ticker, e.g. AAPL. Hong Kong: hk00700. A-shares: sh600519, sz000001.")
}
func (s Strings) LongbridgeToggle() string {
	return s.t("美股使用长桥行情（含盘前、盘后、夜盘）", "Use Longbridge for US quotes (adds pre-market, post-market, overnight)")
}
func (s Strings) LongbridgeHint() string {
	return s.t("需先安装长桥 CLI，并在终端运行 longbridge auth login", "Install the Longbridge CLI and run longbridge auth login first.")
}
func (s Strings) FinnhubKey() string { return "Finnhub API key" }
func (s Strings) FinnhubPlaceholder() string {
	return s.t("留空则美股使用腾讯行情（约 20 秒一次快照）", "Empty: Tencent quotes, about every 20 s")
}
func (s Strings) Cancel() string { return s.t("取消", "Cancel") }
func (s Strings) Save() string   { return s.t("保存", "Save") }

func (s Strings) Unrecognized(entries []string) string {
	return s.t("无法识别："+strings.Join(entries, "、"), "Not recognized: "+strings.Join(entries, ", "))
}

// Percent writes an opacity choice, such as 85%.
func Percent(v float64) string { return fmt.Sprintf("%d%%", int(v*100+0.5)) }
