package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"iOSGhostRun/services"
)

//go:embed web
var webFS embed.FS

// Config 持久化配置
type Config struct {
	Speed         float64          `json:"speed"`
	SpeedVariance float64          `json:"speedVariance"`
	RouteOffset   float64          `json:"routeOffset"`
	Route         []services.Point `json:"route"`
	AutoStart     bool             `json:"autoStart"`
}

// ServerState 服务器运行状态
type ServerState struct {
	Phase      string `json:"phase"` // waiting_device, preparing, running, stopped, error
	DeviceUDID string `json:"deviceUdid"`
	DeviceName string `json:"deviceName"`
	Error      string `json:"error"`
}

var (
	config     Config
	state      ServerState
	stateMu    sync.RWMutex
	configPath string

	devicesSvc  *services.DevicesService
	locationSvc *services.LocationService
	runningSvc  *services.RunningService
	imageSvc    *services.ImageService

	sseDispatcher *services.SSEEventDispatcher
)

func main() {
	port := flag.Int("port", 8080, "HTTP 服务端口")
	flag.Parse()

	// 初始化日志
	services.NewLoggerService()

	// 初始化 SSE 事件分发
	sseDispatcher = services.NewSSEEventDispatcher()
	services.GlobalEvents = sseDispatcher

	// 初始化服务
	devicesSvc = &services.DevicesService{}
	locationSvc = &services.LocationService{}
	runningSvc = services.NewRunningService()
	imageSvc = &services.ImageService{}

	// 加载配置
	exePath, _ := os.Executable()
	configPath = filepath.Join(filepath.Dir(exePath), "config.json")
	loadConfig()

	// 设置状态
	setState("waiting_device", "", "", "")

	// HTTP 路由
	mux := http.NewServeMux()

	// Apple captive portal detection - 让 iOS 认为有网，不会自动断开 WiFi
	mux.HandleFunc("/hotspot-detect.html", handleCaptivePortal)
	mux.HandleFunc("/library/test/success.html", handleCaptivePortal)
	mux.HandleFunc("/generate_204", handleCaptivePortal)

	// API
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/config", handleConfig)
	mux.HandleFunc("/api/route", handleRoute)
	mux.HandleFunc("/api/start", handleStart)
	mux.HandleFunc("/api/stop", handleStop)
	mux.HandleFunc("/api/events", handleSSE)
	mux.HandleFunc("/api/logs", handleLogs)
	mux.HandleFunc("/api/shutdown", handleShutdown)

	// 静态文件
	webContent, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(webContent)))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
	}

	// 打印访问信息
	fmt.Println("========================================")
	fmt.Println("  iOSGhostRun - 树莓派 GPS 模拟器")
	fmt.Println("========================================")
	fmt.Printf("  Web 配置页: http://%s:%d\n", getLocalIP(), *port)
	fmt.Println("  等待 iPhone 连接...")
	fmt.Println("========================================")

	// 启动设备检测循环
	ctx, cancel := context.WithCancel(context.Background())
	go deviceLoop(ctx)

	// 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n正在关闭...")
		cancel()
		runningSvc.StopRun()
		server.Shutdown(context.Background())
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// deviceLoop 自动检测设备并开始跑步
func deviceLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}

		stateMu.RLock()
		phase := state.Phase
		stateMu.RUnlock()

		// 如果正在跑步，检查设备是否还在
		if phase == "running" {
			_, err := devicesSvc.GetSelectedDevice()
			if err != nil {
				services.Log.Warn("Server", "设备断开，停止跑步")
				runningSvc.StopRun()
				setState("waiting_device", "", "", "")
			}
			continue
		}

		// 如果已经停止（手动停止），不自动重启
		if phase == "stopped" {
			continue
		}

		// 检测设备
		devices, err := devicesSvc.ListDevices()
		if err != nil || len(devices) == 0 {
			setState("waiting_device", "", "", "")
			continue
		}

		device := devices[0]
		services.Log.Info("Server", fmt.Sprintf("检测到设备: %s (%s)", device.DeviceName, device.UDID))
		setState("preparing", device.UDID, device.DeviceName, "")

		// 选择设备（挂载镜像 + 启动隧道）
		if err := devicesSvc.SelectDevice(device.UDID); err != nil {
			services.Log.Error("Server", "设备准备失败: "+err.Error())
			setState("error", device.UDID, device.DeviceName, err.Error())
			continue
		}

		// 自动开始跑步
		if config.AutoStart && len(config.Route) >= 2 {
			services.Log.Info("Server", "自动开始跑步")
			runningSvc.SetSpeed(config.Speed)
			runningSvc.SetRandomization(config.SpeedVariance, config.RouteOffset)
			runningSvc.SetLoopCount(0) // 无限循环

			if err := runningSvc.StartRun(device.UDID, config.Route, config.Speed); err != nil {
				services.Log.Error("Server", "启动跑步失败: "+err.Error())
				setState("error", device.UDID, device.DeviceName, err.Error())
				continue
			}
			setState("running", device.UDID, device.DeviceName, "")
		} else {
			if len(config.Route) < 2 {
				setState("error", device.UDID, device.DeviceName, "未配置路线（至少需要2个点），请通过 Web 页面配置")
			} else {
				setState("stopped", device.UDID, device.DeviceName, "")
			}
		}
	}
}

func setState(phase, udid, name, errMsg string) {
	stateMu.Lock()
	state = ServerState{Phase: phase, DeviceUDID: udid, DeviceName: name, Error: errMsg}
	stateMu.Unlock()
	sseDispatcher.Emit("server:state", state)
}

// === HTTP Handlers ===

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stateMu.RLock()
	resp := struct {
		Server  ServerState           `json:"server"`
		Running services.RunningStatus `json:"running"`
		Config  Config                `json:"config"`
	}{
		Server:  state,
		Running: runningSvc.GetStatus(),
		Config:  config,
	}
	stateMu.RUnlock()
	json.NewEncoder(w).Encode(resp)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var c struct {
		Speed         *float64 `json:"speed"`
		SpeedVariance *float64 `json:"speedVariance"`
		RouteOffset   *float64 `json:"routeOffset"`
		AutoStart     *bool    `json:"autoStart"`
	}
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if c.Speed != nil {
		config.Speed = *c.Speed
		runningSvc.SetSpeed(*c.Speed)
	}
	if c.SpeedVariance != nil {
		config.SpeedVariance = *c.SpeedVariance
	}
	if c.RouteOffset != nil {
		config.RouteOffset = *c.RouteOffset
	}
	if c.SpeedVariance != nil || c.RouteOffset != nil {
		runningSvc.SetRandomization(config.SpeedVariance, config.RouteOffset)
	}
	if c.AutoStart != nil {
		config.AutoStart = *c.AutoStart
	}

	saveConfig()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config.Route)
	case http.MethodPost:
		var route []services.Point
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		config.Route = route
		saveConfig()
		services.Log.Info("Server", fmt.Sprintf("路线已更新，%d 个点", len(route)))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stateMu.RLock()
	udid := state.DeviceUDID
	name := state.DeviceName
	stateMu.RUnlock()

	if udid == "" {
		http.Error(w, "没有连接的设备", http.StatusBadRequest)
		return
	}
	if len(config.Route) < 2 {
		http.Error(w, "路线至少需要2个点", http.StatusBadRequest)
		return
	}

	runningSvc.SetSpeed(config.Speed)
	runningSvc.SetRandomization(config.SpeedVariance, config.RouteOffset)
	runningSvc.SetLoopCount(0)

	if err := runningSvc.StartRun(udid, config.Route, config.Speed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setState("running", udid, name, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runningSvc.StopRun()

	stateMu.RLock()
	udid := state.DeviceUDID
	name := state.DeviceName
	stateMu.RUnlock()

	setState("stopped", udid, name, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services.Log.GetLogs())
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := sseDispatcher.Subscribe()
	defer sseDispatcher.Unsubscribe(ch)

	// 发送当前状态
	stateMu.RLock()
	initData, _ := json.Marshal(state)
	stateMu.RUnlock()
	fmt.Fprintf(w, "event: server:state\ndata: %s\n\n", initData)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

// === 配置持久化 ===

func loadConfig() {
	config = Config{
		Speed:         8.0,
		SpeedVariance: 1.0,
		RouteOffset:   3.0,
		AutoStart:     true,
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &config)
	services.Log.Info("Server", fmt.Sprintf("已加载配置: 速度=%.1fkm/h, %d个路线点", config.Speed, len(config.Route)))
}

func saveConfig() {
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)
}

// handleCaptivePortal 响应 iOS/Android 的网络连通性检测
func handleCaptivePortal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "<HTML><HEAD><TITLE>Success</TITLE></HEAD><BODY>Success</BODY></HTML>")
}

func handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "shutting_down"})
	services.Log.Info("Server", "收到关机指令，3秒后关机...")
	go func() {
		time.Sleep(3 * time.Second)
		exec.Command("sudo", "shutdown", "-h", "now").Run()
	}()
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "localhost"
}
