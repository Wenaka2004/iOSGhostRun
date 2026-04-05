#!/bin/bash
# iOSGhostRun 树莓派部署脚本
# 用法: sudo bash setup-pi.sh
#
# 功能:
# 1. 安装 iOSGhostRun 二进制
# 2. 配置 WiFi 热点 (SSID: GhostRun, 密码: 12345678)
# 3. 配置开机自启
# 4. 配置 usbmuxd (iPhone USB 通信)

set -e

SSID="GhostRun"
PASS="12345678"
INSTALL_DIR="/opt/iosghostrun"
BINARY="iOSGhostRun-pi"

echo "========================================"
echo "  iOSGhostRun 树莓派部署"
echo "========================================"

# 检查 root
if [ "$EUID" -ne 0 ]; then
  echo "请使用 sudo 运行此脚本"
  exit 1
fi

# 检查二进制文件
if [ ! -f "./$BINARY" ]; then
  echo "错误: 当前目录未找到 $BINARY"
  echo "请将 $BINARY 放在当前目录后重试"
  exit 1
fi

echo ""
echo "[1/5] 安装依赖..."
apt-get update -qq
apt-get install -y -qq hostapd dnsmasq usbmuxd libimobiledevice-utils

echo ""
echo "[2/5] 安装 iOSGhostRun..."
mkdir -p "$INSTALL_DIR"
cp "./$BINARY" "$INSTALL_DIR/iOSGhostRun"
chmod +x "$INSTALL_DIR/iOSGhostRun"

echo ""
echo "[3/5] 配置 WiFi 热点 (SSID: $SSID, 密码: $PASS)..."

# hostapd 配置
cat > /etc/hostapd/hostapd.conf << EOF
interface=wlan0
driver=nl80211
ssid=$SSID
hw_mode=g
channel=7
wmm_enabled=0
macaddr_acl=0
auth_algs=1
ignore_broadcast_ssid=0
wpa=2
wpa_passphrase=$PASS
wpa_key_mgmt=WPA-PSK
rsn_pairwise=CCMP
EOF

# 设置 hostapd 默认配置
sed -i 's|^#DAEMON_CONF=.*|DAEMON_CONF="/etc/hostapd/hostapd.conf"|' /etc/default/hostapd 2>/dev/null || true

# dnsmasq 配置
cat > /etc/dnsmasq.d/ghostrun.conf << EOF
interface=wlan0
dhcp-range=192.168.4.2,192.168.4.20,255.255.255.0,24h
address=/#/192.168.4.1
EOF

# 静态 IP
cat >> /etc/dhcpcd.conf << EOF

# GhostRun WiFi 热点
interface wlan0
static ip_address=192.168.4.1/24
nohook wpa_supplicant
EOF

echo ""
echo "[4/5] 配置开机自启..."

cat > /etc/systemd/system/iosghostrun.service << EOF
[Unit]
Description=iOSGhostRun GPS 模拟器
After=network.target usbmuxd.service
Wants=usbmuxd.service

[Service]
Type=simple
ExecStart=$INSTALL_DIR/iOSGhostRun -port 80
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable hostapd
systemctl enable dnsmasq
systemctl enable iosghostrun

echo ""
echo "[5/5] 取消 wlan0 的 WiFi 客户端模式..."
systemctl disable wpa_supplicant 2>/dev/null || true

echo ""
echo "========================================"
echo "  部署完成！"
echo "========================================"
echo ""
echo "  WiFi: $SSID / $PASS"
echo "  配置页: http://192.168.4.1"
echo ""
echo "  使用方法:"
echo "  1. 重启树莓派: sudo reboot"
echo "  2. 手机连接 WiFi \"$SSID\""
echo "  3. 浏览器访问 http://192.168.4.1 配置路线"
echo "  4. USB 连接 iPhone，自动开始跑步"
echo ""
echo "  管理命令:"
echo "  查看状态: sudo systemctl status iosghostrun"
echo "  查看日志: sudo journalctl -u iosghostrun -f"
echo "  重启服务: sudo systemctl restart iosghostrun"
echo ""
