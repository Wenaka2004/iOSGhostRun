#!/data/data/com.termux/files/usr/bin/bash
# iOSGhostRun Termux 部署脚本
# 前提：已 root、已安装 Termux、有 OTG 线
# 用法：在 Termux 中运行 bash setup-termux.sh

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
fail() { echo -e "${RED}[✗]${NC} $1"; exit 1; }

WORK_DIR="$HOME/iosghostrun"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ========================================
#  1. 检查 root
# ========================================
echo "========================================="
echo "  iOSGhostRun - Termux 部署"
echo "========================================="

if ! su -c "id" > /dev/null 2>&1; then
    fail "需要 root 权限！请先 root 你的手机"
fi
info "Root 权限检查通过"

# ========================================
#  2. 安装依赖
# ========================================
info "更新软件包..."
pkg update -y

info "安装依赖..."
pkg install -y libusb

# 检查 usbmuxd 是否可用
if pkg list-installed 2>/dev/null | grep -q usbmuxd; then
    info "usbmuxd 已安装"
elif pkg install -y usbmuxd 2>/dev/null; then
    info "usbmuxd 安装成功"
else
    warn "Termux 仓库没有 usbmuxd，将使用静态编译版本"
    warn "请手动安装：参考 https://github.com/libimobiledevice/usbmuxd"
    warn "或者从其他来源获取 usbmuxd arm64 二进制放到 $WORK_DIR/"
fi

# 检查 libimobiledevice 工具（用于调试）
if pkg install -y libimobiledevice 2>/dev/null; then
    info "libimobiledevice 工具安装成功"
else
    warn "libimobiledevice 未安装（非必需，但方便调试）"
fi

# ========================================
#  3. 创建工作目录
# ========================================
mkdir -p "$WORK_DIR"
mkdir -p "$WORK_DIR/devimages"
mkdir -p "$WORK_DIR/pairrecords"
info "工作目录: $WORK_DIR"

# ========================================
#  4. 复制二进制
# ========================================
if [ -f "$SCRIPT_DIR/iOSGhostRun-android" ]; then
    cp "$SCRIPT_DIR/iOSGhostRun-android" "$WORK_DIR/iOSGhostRun"
    chmod +x "$WORK_DIR/iOSGhostRun"
    info "二进制已复制"
elif [ -f "$WORK_DIR/iOSGhostRun" ]; then
    info "二进制已存在"
else
    fail "找不到 iOSGhostRun-android 二进制！请放在与此脚本相同的目录"
fi

# ========================================
#  5. 创建默认配置（如果不存在）
# ========================================
if [ ! -f "$WORK_DIR/config.json" ]; then
    cat > "$WORK_DIR/config.json" << 'CONF'
{
  "speed": 8.0,
  "speedVariance": 1.0,
  "routeOffset": 3.0,
  "autoStart": true,
  "route": []
}
CONF
    info "默认配置已创建（需要通过 Web 页面导入路线）"
else
    info "配置文件已存在，保留原有配置"
fi

# ========================================
#  6. 创建启动脚本
# ========================================
cat > "$WORK_DIR/start.sh" << 'STARTSCRIPT'
#!/data/data/com.termux/files/usr/bin/bash
# iOSGhostRun 启动脚本

WORK_DIR="$HOME/iosghostrun"
PORT=8080

echo "======================================"
echo "  iOSGhostRun - Android 启动"
echo "======================================"

# 1. 确保 /var/run 目录存在（root）
su -c "mkdir -p /var/run" 2>/dev/null

# 2. 停止旧的 usbmuxd（如果有）
su -c "killall usbmuxd" 2>/dev/null
sleep 1

# 3. 启动 usbmuxd（root 权限）
echo "[*] 启动 usbmuxd..."
su -c "usbmuxd -f -v" &
USBMUXD_PID=$!
sleep 2

# 检查 usbmuxd 是否在运行
if su -c "ls /var/run/usbmuxd" > /dev/null 2>&1; then
    echo "[✓] usbmuxd 已启动，socket: /var/run/usbmuxd"
else
    echo "[!] usbmuxd socket 未创建，尝试替代路径..."
    # 某些系统 socket 在其他位置
    for sock in /var/run/usbmuxd /tmp/usbmuxd; do
        if [ -e "$sock" ]; then
            echo "[✓] 找到 socket: $sock"
            break
        fi
    done
fi

# 4. 检查是否能发现 iPhone
echo "[*] 检查 iPhone 连接..."
if command -v idevice_id > /dev/null 2>&1; then
    DEVICES=$(idevice_id -l 2>/dev/null)
    if [ -n "$DEVICES" ]; then
        echo "[✓] 发现设备: $DEVICES"
    else
        echo "[!] 未发现设备，请确认 OTG 线已连接 iPhone 并点击「信任此电脑」"
    fi
fi

# 5. 启动 iOSGhostRun
echo "[*] 启动 iOSGhostRun (端口: $PORT)..."
echo "[*] Web 页面: http://localhost:$PORT"
echo "[*] 热点模式: 开启安卓热点后，iPhone 访问 http://192.168.43.1:$PORT"
echo "======================================"

cd "$WORK_DIR"
./iOSGhostRun -port "$PORT"
STARTSCRIPT
chmod +x "$WORK_DIR/start.sh"
info "启动脚本已创建: $WORK_DIR/start.sh"

# ========================================
#  7. 创建停止脚本
# ========================================
cat > "$WORK_DIR/stop.sh" << 'STOPSCRIPT'
#!/data/data/com.termux/files/usr/bin/bash
echo "[*] 停止 iOSGhostRun..."
killall iOSGhostRun 2>/dev/null
echo "[*] 停止 usbmuxd..."
su -c "killall usbmuxd" 2>/dev/null
echo "[✓] 已停止"
STOPSCRIPT
chmod +x "$WORK_DIR/stop.sh"
info "停止脚本已创建: $WORK_DIR/stop.sh"

# ========================================
#  8. 配置 Termux:Boot 自启动（可选）
# ========================================
BOOT_DIR="$HOME/.termux/boot"
mkdir -p "$BOOT_DIR"
cat > "$BOOT_DIR/iosghostrun.sh" << 'BOOTSCRIPT'
#!/data/data/com.termux/files/usr/bin/bash
# Termux:Boot 自启动脚本
# 需要安装 Termux:Boot 插件才能生效
termux-wake-lock
sleep 5
exec $HOME/iosghostrun/start.sh
BOOTSCRIPT
chmod +x "$BOOT_DIR/iosghostrun.sh"
info "Termux:Boot 自启动已配置（需安装 Termux:Boot 插件）"

# ========================================
#  完成
# ========================================
echo ""
echo "========================================="
echo "  部署完成！"
echo "========================================="
echo ""
echo "  使用方法："
echo "  1. 用 OTG 线连接 iPhone"
echo "  2. iPhone 上点击「信任此电脑」"
echo "  3. 运行: ~/iosghostrun/start.sh"
echo "  4. 开启安卓热点"
echo "  5. iPhone 连接热点后访问: http://192.168.43.1:$PORT"
echo ""
echo "  停止: ~/iosghostrun/stop.sh"
echo ""
echo "  自启动（需 Termux:Boot 插件）："
echo "  安装后重启手机即自动运行"
echo ""
echo "  导入路线："
echo "  Web 页面 → 导入 JSON → 选择路线文件"
echo "========================================="
