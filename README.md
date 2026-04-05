# iOSGhostRun

iOS GPS 虚拟定位模拟器，支持 iOS 17+ 设备。可用于模拟跑步轨迹、测试位置相关应用等场景。

基于 [go-ios](https://github.com/danielpaulus/go-ios) 和 [Wails v3](https://wails.io) 开发。

## 功能特性

- **跨平台支持**：Windows 桌面版、Raspberry Pi 无头版、Android Termux 版
- **iOS 17+ 隧道支持**：自动处理 iOS 17+ 的 RSD 隧道连接
- **真实感 GPS 模拟**：
  - 多频率速度波动模拟真实跑步节奏
  - GPS 信号漂移模拟（±1-2米）
  - 路线偏移避免轨迹过于规整
  - 更新间隔抖动（900-1100ms）模拟真实 GPS 采样
- **Web 配置界面**：通过浏览器配置速度、导入路线
- **WiFi 热点模式**：树莓派可广播 WiFi 热点，手机连接后配置
- **一键启动**：支持 Termux 快捷方式和树莓派开机自启

## 截图

### Windows

![Windows 截图](assets/screenshot_windows.png)

### macOS

![macOS 截图](assets/screenshot_macos.png)

---

## 系统要求

### iOS 设备
- iOS 16 及以上
- 需要信任此电脑（配对）
- iOS 17+ 会自动使用隧道模式

### Windows 桌面版
- Windows 10/11
- [Go 1.21+](https://go.dev/dl/)
- [Bun](https://bun.sh/) 或 Node.js（用于前端构建）

### Raspberry Pi 版
- Raspberry Pi 3/4/5（64位系统）
- 或 Pi Zero 2 W

### Android Termux 版
- 已 root 的 Android 手机
- OTG 数据线（连接 iPhone）
- [Termux](https://termux.dev/) 应用

---

## 使用说明

### 一、Windows 桌面版

#### 1. 安装依赖

```bash
# 安装 Go 1.21+
# 安装 Bun: https://bun.sh

# 克隆项目
git clone https://github.com/Wenaka2004/iOSGhostRun.git
cd iOSGhostRun
```

#### 2. 运行开发模式

```bash
# 安装前端依赖
cd frontend && bun install && cd ..

# 启动开发服务器
wails3 dev
```

#### 3. 构建生产版本

```bash
wails3 build
```

可执行文件位于 `build/windows/` 目录。

#### 4. 使用

1. USB 连接 iPhone 到电脑
2. iPhone 上点击「信任此电脑」并输入密码
3. 运行程序，在地图上绘制路线
4. 设置速度参数，点击「开始跑步」

---

### 二、Raspberry Pi 版（无头模式）

Pi 版本适合放在包里，通电后自动开始跑步，通过手机连接 WiFi 热点配置。

#### 1. 交叉编译

在 Windows 上编译 ARM64 二进制：

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags headless -trimpath -ldflags="-s -w" -o bin/iOSGhostRun-pi ./cmd/server
```

#### 2. 部署到 Pi

将以下文件传到 Pi：
- `bin/iOSGhostRun-pi`
- `deploy/setup-pi.sh`

在 Pi 上运行：

```bash
chmod +x setup-pi.sh
sudo bash setup-pi.sh
```

脚本会自动：
- 安装 usbmuxd、hostapd、dnsmasq
- 配置 WiFi 热点（SSID: `GhostRun`，密码: `12345678`）
- 创建 systemd 服务实现开机自启

#### 3. 使用

1. USB 连接 iPhone 到 Pi
2. Pi 通电后自动启动服务
3. 手机连接 `GhostRun` WiFi 热点
4. 浏览器访问 `http://192.168.4.1` 配置

**启动/停止服务：**

```bash
sudo systemctl start iosghostrun   # 启动
sudo systemctl stop iosghostrun    # 停止
sudo systemctl status iosghostrun  # 状态
```

**导出路线：**

在 Windows 桌面版中导出路线 JSON，然后在 Pi 的 Web 界面导入。

---

### 三、Android Termux 版

使用已 root 的安卓手机通过 OTG 线给 iPhone 虚拟定位。

#### 1. 前提条件

- 安卓手机已 root
- 已安装 [Termux](https://f-droid.org/packages/com.termux/)
- OTG 数据线（安卓 → iPhone）

#### 2. 安装依赖

在 Termux 中：

```bash
# 更新软件包
pkg update

# 安装必要工具
pkg install libusb libimobiledevice
```

#### 3. 获取二进制

方法一：下载预编译版本

从项目的 Releases 页面下载 `iOSGhostRun-android`。

方法二：自行编译（需要 Go）

```bash
pkg install golang
git clone https://github.com/Wenaka2004/iOSGhostRun.git
cd iOSGhostRun
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags headless -o iOSGhostRun-android ./cmd/server
```

#### 4. 部署

```bash
mkdir -p ~/iosghostrun
mv iOSGhostRun-android ~/iosghostrun/iOSGhostRun
chmod +x ~/iosghostrun/iOSGhostRun
```

#### 5. 配对 iPhone

首次使用需要配对：

```bash
# 启动 usbmuxd
USBMUXD=$(which usbmuxd)
su -c "$USBMUXD" &

# 配对
idevicepair pair
```

iPhone 上会弹出「信任此电脑」，点击信任并输入密码。

#### 6. 启动服务

创建启动脚本：

```bash
cat > ~/iosghostrun/start.sh << 'EOF'
#!/data/data/com.termux/files/usr/bin/bash
killall iOSGhostRun 2>/dev/null
su -c "killall -9 usbmuxd" 2>/dev/null
sleep 1

USBMUXD_BIN=$(which usbmuxd)
su -c "$USBMUXD_BIN" &
sleep 3

export USBMUXD_SOCKET_ADDRESS=/data/data/com.termux/files/usr/var/run/usbmuxd
cd ~/iosghostrun
exec ./iOSGhostRun -port 8080
EOF
chmod +x ~/iosghostrun/start.sh
```

运行：

```bash
bash ~/iosghostrun/start.sh
```

#### 7. 访问 Web 界面

- 安卓浏览器访问：`http://localhost:8080`
- 或开启安卓热点，iPhone 连接后访问：`http://192.168.43.1:8080`

#### 8. 一键启动（可选）

安装 [Termux:Widget](https://f-droid.org/packages/com.termux.widget/) 后：

```bash
mkdir -p ~/.shortcuts
cat > ~/.shortcuts/GhostRun启动 << 'EOF'
#!/bin/bash
bash ~/iosghostrun/start.sh
EOF
chmod +x ~/.shortcuts/GhostRun启动
```

桌面添加 Termux 小部件即可一键启动。

---

## Web 界面功能

访问 `http://localhost:8080` 或 `http://192.168.4.1`（Pi 热点）可使用：

| 功能 | 说明 |
|------|------|
| 状态显示 | 当前运行状态、设备名称、跑步进度 |
| 速度配置 | 设置基准速度和速度波动范围 |
| 路线管理 | 导入/导出 JSON 格式路线 |
| 启动/停止 | 手动控制跑步 |
| 日志查看 | 实时查看运行日志 |
| 关机按钮 | 树莓派专用，点击后关机 |

### 路线 JSON 格式

```json
[
  {"lat": xx.xxxx, "lon": xxx.xxxx},
  {"lat": xx.xxxx, "lon": xxx.xxxx},
  {"lat": xx.xxxx, "lon": xxx.xxxx}
]
```

至少需要 2 个点，程序会按顺序移动并循环。

---

## 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| speed | 基准速度 (km/h) | 8.0 |
| speedVariance | 速度波动范围 (km/h) | 1.0 |
| routeOffset | 路线偏移距离 (m) | 3.0 |
| autoStart | 插入设备后自动开始 | true |

---

## 常见问题

### Q: iOS 18 报 DVTSecureSocketProxy InvalidService 错误？

A: iOS 17+ 需要使用隧道模式连接设备。程序已自动处理，会检测 iOS 版本并切换到隧道方式。

### Q: 跑步软件不计里程？

A: 确保速度设置合理（5-12 km/h），GPS 更新频率已优化为 1 秒，模拟真实 GPS 采样。

### Q: WiFi 热点连上后自动断开？

A: iOS 会检测网络连通性，无网就断开。程序已内置 captive portal 响应，返回「Success」让 iOS 认为有网。

### Q: Termux 报 "usbmuxd: not found"？

A: usbmuxd 在 Termux 环境下需要用完整路径：

```bash
$(which usbmuxd)
```

### Q: Termux 报 "/var/run/usbmuxd: no such file or directory"？

A: Android 的 /var 是只读的。设置环境变量：

```bash
export USBMUXD_SOCKET_ADDRESS=/data/data/com.termux/files/usr/var/run/usbmuxd
```

### Q: GPS 信号强度低，跑步软件不开始？

A: 虚拟定位无法伪造 GPS 信号强度。解决方法：
1. 到窗边或室外让 iPhone 搜到真实卫星信号
2. 跑步软件判定 GPS OK 开始记录后，虚拟定位会接管

### Q: 如何获取路线坐标？

A:
1. 在 Google Maps 上右键点击获取坐标
2. 使用 Windows 桌面版在地图上绘制
3. 从其他地图工具导出 GPX 后转换为 JSON

---

## 测试环境

| 平台 | 操作系统 | 设备 |
|------|----------|------|
| Windows | Windows 11 | AMD64 |
| macOS | macOS 15.7 | x64 |
| Raspberry Pi | Raspberry Pi OS 64-bit | Pi 3/4 |
| Android | Termux | Rooted ARM64 |
| iOS | iOS 16.1 - iOS 18 | iPhone |

---

## 技术栈

- **后端**: Go + [go-ios](https://github.com/danielpaulus/go-ios)
- **前端**: Vue 3 + Vite + OpenLayers
- **桌面框架**: Wails v3
- **iOS 通信**: usbmuxd + lockdown + instruments

---

## 许可证与免责声明

[MIT License](LICENSE)

**免责声明**: 本软件仅供学习研究使用，使用本工具模拟位置信息可能违反某些应用的服务条款，请自行承担风险。作者不对因使用本工具造成的任何后果负责。
