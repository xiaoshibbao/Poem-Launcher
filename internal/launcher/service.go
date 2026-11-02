//go:build windows

package launcher

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"poem/internal/injector"
	"poem/internal/netproxy"
)

// launchProfile is set only in the separate module-isolation diagnostic build.
var launchProfile = "combined"

type Settings struct {
	LoginChannel       string   `json:"loginChannel"`
	LuaEnabled         bool     `json:"luaEnabled"`
	LuaScripts         []string `json:"luaScripts"`
	LuaDisabledScripts []string `json:"luaDisabledScripts"`
	RemoveBlur         bool     `json:"removeBlur"`
	TextLanguage       string   `json:"textLanguage"`
	VoiceLanguage      string   `json:"voiceLanguage"`
	GamePath           string   `json:"gamePath"`
	ProxyEnabled       bool     `json:"proxyEnabled"`
	ServerURL          string   `json:"serverURL"`
	ProxyPort          int      `json:"proxyPort"`
	RSAPatch           bool     `json:"rsaPatch"`
	RSAKey             string   `json:"rsaKey"`
	WebRedirect        bool     `json:"webRedirect"`
	WebHosts           string   `json:"webHosts"`
	TimeStop           bool     `json:"timeStop"`
	TimeStopKey        int      `json:"timeStopKey"`
	UnlockFPS          bool     `json:"unlockFPS"`
	TargetFPS          int      `json:"targetFPS"`
	MultiInstance      bool     `json:"multiInstance"`
	TouchUI            bool     `json:"touchUI"`
	Instances          int      `json:"instances"`
}

type Status struct {
	Running int    `json:"running"`
	Ports   []int  `json:"ports"`
	Message string `json:"message"`
}

type Service struct {
	hook, features []byte
	blur           []byte
	network        []byte
	lua            []byte
	luaSessions    map[uint32]string
	luaLoaded      map[uint32]bool
	blurControl    *blurControl
	mu             sync.Mutex
	saveMu         sync.Mutex
	sessions       map[uint32]int
	sessionModes   map[uint32]struct{ Channel, Game string }
}

// Modules holds the native DLLs embedded in the launcher executable.
type Modules struct {
	Proxy    []byte // ProxyHook.dll: private-server traffic hook
	Features []byte // GameFeatures.dll: time stop, FPS, multi-instance, touch UI
	Visual   []byte // VisualPatch.dll: character de-blur
	Network  []byte // NetworkEntry.dll: native network bridge
	Lua      []byte // LuaEntry.dll: local Lua bridge
}

func New(m Modules) *Service {
	return &Service{
		hook: m.Proxy, features: m.Features, blur: m.Visual, network: m.Network, lua: m.Lua,
		sessions: map[uint32]int{}, sessionModes: map[uint32]struct{ Channel, Game string }{},
		luaSessions: map[uint32]string{}, luaLoaded: map[uint32]bool{},
	}
}

func defaults() Settings {
	return Settings{ServerURL: "http://127.0.0.1:21000", ProxyEnabled: true, ProxyPort: 8080, RSAPatch: true, WebRedirect: true, TimeStop: true, UnlockFPS: true, MultiInstance: true, TimeStopKey: 0x20, TargetFPS: 120, Instances: 1}
}

func normalizeSettings(cfg Settings) Settings {
	cfg.Instances = 1
	return cfg
}

func dataDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(exe), "Poem-data")
	return dir, os.MkdirAll(dir, 0755)
}

func (s *Service) LoadSettings() (Settings, error) {
	dir, err := dataDir()
	if err != nil {
		return Settings{}, err
	}
	cfg := defaults()
	b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(b, &cfg)
	return normalizeSettings(cfg), err
}

func validate(cfg Settings, launching bool) error {
	for _, language := range []string{cfg.TextLanguage, cfg.VoiceLanguage} {
		if language != "" && language != "cn" && language != "en" && language != "jp" && language != "kr" {
			return errors.New("语言仅支持中文、英文、日文和韩文")
		}
	}
	if cfg.ProxyPort < 0 || cfg.ProxyPort > 65535 {
		return errors.New("代理端口须在 0–65535 之间")
	}
	if cfg.TargetFPS < 30 || cfg.TargetFPS > 1000 {
		return errors.New("帧率上限须在 30–1000 之间")
	}
	if cfg.TimeStopKey < 1 || cfg.TimeStopKey > 254 {
		return errors.New("请设置有效的时停按键")
	}
	if cfg.Instances < 1 || cfg.Instances > 8 {
		return errors.New("启动数量须在 1–8 之间")
	}
	if cfg.Instances > 1 && !cfg.MultiInstance {
		return errors.New("启动多个窗口前请启用解除多开限制")
	}
	if cfg.ProxyEnabled {
		u, err := url.Parse(strings.TrimSpace(cfg.ServerURL))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
			return errors.New("请输入完整的 HTTP/HTTPS 服务器地址，不含用户名、查询或片段")
		}
		if cfg.RSAPatch {
			if _, err := netproxy.NormalizeRSAKey(cfg.RSAKey); err != nil {
				return err
			}
		}
	}
	if launching {
		if !strings.EqualFold(filepath.Base(cfg.GamePath), "StarRail.exe") {
			return errors.New("请选择游戏目录中的 StarRail.exe")
		}
		if info, err := os.Stat(cfg.GamePath); err != nil || info.IsDir() {
			return errors.New("游戏程序不存在，请重新选择路径")
		}
		if info, err := os.Stat(filepath.Join(filepath.Dir(cfg.GamePath), "StarRail_Data")); err != nil || !info.IsDir() {
			return errors.New("所选路径缺少 StarRail_Data 游戏目录")
		}
	}
	return nil
}

func (s *Service) SaveSettings(cfg Settings) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	cfg = normalizeSettings(cfg)
	dir, err := dataDir()
	if err != nil {
		return err
	}
	cfg.ServerURL = strings.TrimSpace(cfg.ServerURL)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "settings.json"), b, 0600)
}

func (s *Service) GetStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked("")
}

func (s *Service) statusLocked(message string) Status {
	ports := []int{}
	for _, p := range s.sessions {
		if p > 0 {
			ports = append(ports, p)
		}
	}
	return Status{Running: len(s.sessions), Ports: ports, Message: message}
}

func writeModule(path string, b []byte) error {
	if len(b) == 0 {
		return fmt.Errorf("缺少功能模块 %s", filepath.Base(path))
	}
	existing, err := os.ReadFile(path)
	if err == nil && sha256.Sum256(existing) == sha256.Sum256(b) {
		return nil
	}
	return os.WriteFile(path, b, 0644)
}

// The game-features DLL reads <game>/StarRail_Data/config.ini synchronously in DllMain.
// Temporarily merge just our keys, then restore the original bytes after loading.
func featureConfig(cfg Settings) string {
	boolean := func(v bool) string {
		if v {
			return "True"
		}
		return "False"
	}
	return fmt.Sprintf("CompatibilityMode=False\nUnlockMultiInstance=%s\nEnableTouchUI=%s\nEnableBattleTimeStop=%s\nBattleTimeStopKey=0x%X\nUnlockFPS=%s\nTargetFPS=%d\n", boolean(cfg.MultiInstance), boolean(cfg.TouchUI), boolean(cfg.TimeStop), cfg.TimeStopKey, boolean(cfg.UnlockFPS), cfg.TargetFPS)
}

func stageConfig(path string, cfg Settings) (func() error, error) {
	old, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	keys := map[string]bool{"CompatibilityMode": true, "UnlockMultiInstance": true, "EnableTouchUI": true, "EnableBattleTimeStop": true, "BattleTimeStopKey": true, "UnlockFPS": true, "TargetFPS": true}
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(old), "\r\n", "\n"), "\n") {
		key, _, _ := strings.Cut(line, "=")
		if !keys[strings.TrimSpace(key)] {
			lines = append(lines, line)
		}
	}
	merged := strings.Join(lines, "\n") + "\n" + featureConfig(cfg)
	if err := os.WriteFile(path, []byte(merged), 0644); err != nil {
		return nil, err
	}
	return func() error {
		if exists {
			return os.WriteFile(path, old, 0644)
		}
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}, nil
}

// Start starts a batch serially so every module reads the intended settings.
func (s *Service) Start(cfg Settings) (Status, error) {
	if cfg.LoginChannel != "" && cfg.LoginChannel != "private" && cfg.LoginChannel != "official" && cfg.LoginChannel != "bilibili" {
		return Status{}, errors.New("未知登录渠道")
	}
	cfg = normalizeSettings(cfg)
	savedSettings := cfg
	cfg, retail := channelRuntimeSettings(cfg)
	if err := validate(cfg, true); err != nil {
		return Status{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) > 0 && !cfg.MultiInstance {
		return s.statusLocked(""), errors.New("游戏已在运行；继续启动需要解除多开限制")
	}
	for _, existing := range s.sessionModes {
		if existing.Channel != cfg.LoginChannel || !strings.EqualFold(existing.Game, cfg.GamePath) {
			return s.statusLocked(""), errors.New("多开请使用相同渠道和游戏目录；切换渠道前请退出已有游戏")
		}
	}
	if err := s.SaveSettings(savedSettings); err != nil {
		return s.statusLocked(""), err
	}
	dir, err := dataDir()
	if err != nil {
		return s.statusLocked(""), err
	}
	if err = applyLanguage(cfg, dir, len(s.sessions) > 0); err != nil {
		return s.statusLocked(""), err
	}
	if retail {
		if len(s.sessions) == 0 {
			err = prepareRetailChannel(cfg, dir)
		}
	} else {
		err = restorePrivateChannel(cfg.GamePath, dir)
	}
	if err != nil {
		return s.statusLocked(""), err
	}
	moduleDir, err := os.MkdirTemp("", "Poem-runtime-")
	if err != nil {
		return s.statusLocked(""), err
	}
	keepRuntime := false
	defer func() {
		if !keepRuntime {
			cleanupRuntimeModules(moduleDir)
		}
	}()
	featurePath := filepath.Join(moduleDir, "Poem.Game.dll")
	hookPath := filepath.Join(moduleDir, "Poem.Proxy.dll")
	hookBytes := s.hook
	if cfg.ProxyEnabled {
		hookBytes, err = configureProxyThreadNotifications(hookBytes)
		if err != nil {
			return s.statusLocked(""), err
		}
	}
	if cfg.ProxyEnabled && cfg.RSAPatch {
		hookBytes, err = configureHookRSA(hookBytes, cfg.RSAKey)
		if err != nil {
			return s.statusLocked(""), err
		}
		hash := sha256.Sum256(hookBytes)
		hookPath = filepath.Join(moduleDir, fmt.Sprintf("Poem.Proxy-%x.dll", hash[:8]))
	}
	featuresOn := launchProfile != "proxy-only" && (cfg.MultiInstance || cfg.TouchUI || cfg.TimeStop || cfg.UnlockFPS)
	if featuresOn {
		featureBytes, patchErr := configureFeatureThreadExit(s.features)
		if patchErr != nil {
			return s.statusLocked(""), patchErr
		}
		if err = writeModule(featurePath, featureBytes); err != nil {
			return s.statusLocked(""), err
		}
	}
	if cfg.ProxyEnabled {
		if err = writeModule(hookPath, hookBytes); err != nil {
			return s.statusLocked(""), err
		}
	}
	blurPath := filepath.Join(moduleDir, "Poem.Visual.dll")
	var blurLease *blurControl
	if cfg.RemoveBlur {
		if err = writeModule(blurPath, s.blur); err != nil {
			return s.statusLocked(""), err
		}
		blurFPS := cfg.TargetFPS
		if !cfg.UnlockFPS {
			blurFPS = 60
		}
		if s.blurControl != nil && s.blurControl.fps != blurFPS {
			return s.statusLocked(""), errors.New("去虚化多开窗口共用帧率，请保持相同帧率或先退出已有游戏")
		}
		if s.blurControl == nil {
			s.blurControl, err = newBlurControl(blurFPS)
			if err != nil {
				return s.statusLocked(""), err
			}
		}
		blurLease = s.blurControl
		defer func() {
			if blurLease.refs == 0 {
				blurLease.close()
				s.blurControl = nil
			}
		}()
	}
	for n := 0; n < cfg.Instances; n++ {
		sessionDir := filepath.Join(dir, "sessions", fmt.Sprintf("%d-%d", time.Now().UnixNano(), n))
		env := proxyRuntimeEnvironment(cfg.ProxyEnabled)
		dlls := []string{}
		port := 0
		var proxy *netproxy.Proxy
		if cfg.ProxyEnabled {
			if err = os.MkdirAll(sessionDir, 0755); err != nil {
				return s.statusLocked(""), err
			}
			proxy, err = netproxy.New(strings.TrimSpace(cfg.ServerURL), netproxy.PatchOptions{RSAPatch: cfg.RSAPatch, RSAKey: cfg.RSAKey, WebRedirect: cfg.WebRedirect, WebHosts: cfg.WebHosts})
			if err != nil {
				return s.statusLocked(""), err
			}
			ec2b := filepath.Join(sessionDir, "ClientSecretKey.bin")
			proxy.SetEc2bFile(ec2b)
			if err = proxy.CA().InstallTrust(); err != nil {
				return s.statusLocked(""), fmt.Errorf("代理证书安装失败：%w", err)
			}
			port, err = proxy.Start(cfg.ProxyPort)
			if err != nil {
				return s.statusLocked(""), err
			}
			env[hookEnvProxy] = fmt.Sprintf("127.0.0.1:%d", port)
			if sourcePort := systemHTTPProxyPort(); sourcePort != 0 && sourcePort != port && len(s.network) > 0 {
				env["POEM_SYSTEM_PROXY_PORT"] = strconv.Itoa(sourcePort)
				env["POEM_HTTP_PROXY_PORT"] = strconv.Itoa(port)
				writeLaunchLog(dir, "process-local proxy entry routing enabled sourcePort=%d poemPort=%d", sourcePort, port)
			}
			setGameHTTPProxy(env, port)
			proxy.SetLogger(func(format string, args ...any) { writeLaunchLog(dir, format, args...) })
			writeLaunchLog(dir, "game HTTP proxy environment=%s", env["HTTPS_PROXY"])
			env[hookEnvEc2bFile] = ec2b
			env[hookEnvWebTarget] = strings.TrimSpace(cfg.ServerURL)
			dlls = append(dlls, hookPath)
		}
		var restore func() error
		if featuresOn {
			featureSettings := cfg
			restore, err = stageConfig(filepath.Join(filepath.Dir(cfg.GamePath), "StarRail_Data", "config.ini"), featureSettings)
			if err != nil {
				if proxy != nil {
					proxy.Stop()
				}
				return s.statusLocked(""), fmt.Errorf("写入游戏功能配置失败：%w", err)
			}
			dlls = append([]string{featurePath}, dlls...)
		}
		writeLaunchLog(dir, "profile=%s game=%s modules=%v proxyPort=%d", launchProfile, cfg.GamePath, dlls, port)
		var proc *injector.Process
		var launchErr error
		early := []string{}
		late := []string{}
		if env["POEM_SYSTEM_PROXY_PORT"] != "" {
			networkPath := filepath.Join(moduleDir, "Poem.Network.dll")
			if err := writeModule(networkPath, s.network); err != nil {
				if proxy != nil {
					proxy.Stop()
				}
				if restore != nil {
					_ = restore()
				}
				return s.statusLocked(""), err
			}
			early = append(early, networkPath)
		}
		if featuresOn {
			early = append(early, featurePath)
		}
		if cfg.ProxyEnabled {
			late = append(late, hookPath)
		}
		if cfg.RemoveBlur {
			// The original unlocker loads this module before the main thread.
			// Never initialise its protected loader after the proxy hook starts running.
			early = append(early, blurPath)
		}
		releaseDumps, dumpErr := acquireCrashDumps(dir, cfg.GamePath)
		if dumpErr != nil {
			if restore != nil {
				_ = restore()
			}
			if proxy != nil {
				proxy.Stop()
			}
			return s.statusLocked(""), dumpErr
		}
		if fullCrashDiagnostics == "true" {
			writeLaunchLog(dir, "full crash dumps enabled folder=%s", filepath.Join(dir, "crash-dumps"))
		}
		if cfg.ProxyEnabled {
			releaseRoute, routeErr := acquireServerDirectRoute(cfg.ServerURL, func(format string, args ...any) { writeLaunchLog(dir, format, args...) })
			if routeErr != nil {
				releaseDumps()
				if restore != nil {
					_ = restore()
				}
				if proxy != nil {
					proxy.Stop()
				}
				return s.statusLocked(""), routeErr
			}
			releaseCapture := releaseDumps
			releaseDumps = func() { releaseRoute(); releaseCapture() }
			index, bindErr := serverPhysicalInterface(cfg.ServerURL)
			if bindErr == nil && index != 0 {
				bindErr = proxy.BindServerInterface(index, func(format string, args ...any) { writeLaunchLog(dir, format, args...) })
				if bindErr == nil {
					writeLaunchLog(dir, "server sockets pinned interface=%d", index)
				}
			}
			if bindErr != nil {
				releaseDumps()
				if restore != nil {
					_ = restore()
				}
				if proxy != nil {
					proxy.Stop()
				}
				return s.statusLocked(""), bindErr
			}
		}
		if len(early) > 0 && len(late) > 0 || cfg.RemoveBlur {
			writeLaunchLog(dir, "staged early=%v late=%v foregroundGC=%t", early, late, cfg.ProxyEnabled)
			proc, launchErr = injector.LaunchStaged(cfg.GamePath, early, late, env)
		} else {
			proc, launchErr = injector.LaunchWithDLLs(cfg.GamePath, dlls, env)
		}
		if launchErr != nil {
			writeLaunchLog(dir, "launch error: %v", launchErr)
		} else {
			writeLaunchLog(dir, "process resumed pid=%d", proc.Pid())
		}
		var restoreErr error
		if restore != nil {
			restoreErr = restore()
		}
		if launchErr != nil {
			releaseDumps()
			if proxy != nil {
				proxy.Stop()
			}
			return s.statusLocked(""), fmt.Errorf("启动失败：%w", launchErr)
		}
		if executable, err := os.Executable(); err == nil {
			recorder := exec.Command(executable, "--poem-console-recorder", strconv.FormatUint(uint64(proc.Pid()), 10), filepath.Join(dir, "console.log"))
			recorder.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
			if err := recorder.Start(); err != nil {
				writeLaunchLog(dir, "console recorder failed: %v", err)
			} else {
				go recorder.Wait()
			}
		}
		s.sessions[proc.Pid()] = port
		s.sessionModes[proc.Pid()] = struct{ Channel, Game string }{cfg.LoginChannel, cfg.GamePath}
		s.luaSessions[proc.Pid()] = moduleDir
		if cfg.LuaEnabled && len(enabledLuaScripts(cfg)) > 0 {
			pid := proc.Pid()
			paths := enabledLuaScripts(cfg)
			go func() {
				if _, luaErr := s.queueLua(pid, paths, true); luaErr != nil {
					writeLaunchLog(dir, "automatic Lua pid=%d error=%v", pid, luaErr)
					_ = os.WriteFile(filepath.Join(moduleDir, "lua-status.txt"), []byte("0\nerror\n"+luaErr.Error()), 0600)
				}
			}()
		}
		if blurLease != nil {
			blurLease.refs++
		}
		keepRuntime = true
		cleanupLegacyModules(filepath.Join(dir, "modules"), s.features, s.hook, hookBytes, s.blur, s.network)
		stopTrace := func() {}
		if fullCrashDiagnostics == "true" && cfg.ProxyEnabled {
			if traceStop, traceErr := injector.StartProxyThreadTrace(proc, filepath.Join(dir, fmt.Sprintf("thread-trace-%d-%d.log", proc.Pid(), time.Now().UnixNano()))); traceErr == nil {
				stopTrace = traceStop
			} else {
				writeLaunchLog(dir, "thread trace failed pid=%d: %v", proc.Pid(), traceErr)
			}
		}
		writeLaunchLog(dir, "launcher pid=%d gamePid=%d featureThreadExitPatch=%t proxyAllocationResultPatch=%t proxyProtectionBasePatch=%t", os.Getpid(), proc.Pid(), featuresOn, cfg.ProxyEnabled, cfg.ProxyEnabled)
		go s.monitor(proc, proxy, sessionDir, blurLease, moduleDir, releaseDumps, stopTrace)
		if restoreErr != nil {
			return s.statusLocked("游戏已启动，但配置恢复失败"), fmt.Errorf("游戏已启动，原配置恢复失败：%w", restoreErr)
		}
	}
	status := s.statusLocked("游戏已启动")
	application.Get().Event.Emit("poem:status", status)
	return status, nil
}

func writeLaunchLog(dir, format string, args ...any) {
	f, err := os.OpenFile(filepath.Join(dir, "launch.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func (s *Service) monitor(proc *injector.Process, proxy *netproxy.Proxy, dir string, blurLease *blurControl, moduleDir string, releaseDumps func(), stopTrace func()) {
	defer releaseDumps()
	proc.Wait()
	stopTrace()
	pid := proc.Pid()
	exitCode, exitErr := proc.ExitCode()
	writeLaunchLog(filepath.Dir(filepath.Dir(dir)), "process exited pid=%d code=0x%08X error=%v", pid, exitCode, exitErr)
	proc.Close()
	if luaState := readLuaStatus(pid, moduleDir); luaState.State != "idle" {
		writeLaunchLog(filepath.Dir(filepath.Dir(dir)), "Lua final pid=%d job=%s state=%s message=%s", pid, luaState.ID, luaState.State, luaState.Message)
	}
	cleanupRuntimeModules(moduleDir)
	if proxy != nil {
		proxy.Stop()
	}
	// Session directories are created by Poem and contain only its key file.
	_ = os.Remove(filepath.Join(dir, "ClientSecretKey.bin"))
	_ = os.Remove(filepath.Join(dir, "ClientSecretKey.bin.tmp"))
	_ = os.Remove(dir)
	s.mu.Lock()
	delete(s.sessions, pid)
	delete(s.sessionModes, pid)
	delete(s.luaSessions, pid)
	delete(s.luaLoaded, pid)
	if blurLease != nil {
		blurLease.refs--
		if blurLease.refs == 0 {
			blurLease.close()
			if s.blurControl == blurLease {
				s.blurControl = nil
			}
		}
	}
	exitMessage := "游戏已退出"
	if exitErr == nil && exitCode != 0 {
		exitMessage = fmt.Sprintf("游戏异常退出（0x%08X），详情已记录到 launch.log", exitCode)
	}
	status := s.statusLocked(exitMessage)
	s.mu.Unlock()
	application.Get().Event.Emit("poem:status", status)
}
