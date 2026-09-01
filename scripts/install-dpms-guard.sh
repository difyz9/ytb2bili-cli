#!/bin/bash
SERVICE_PATH="/etc/systemd/system/dpms-keep-active.service"

cat > $SERVICE_PATH << EOF
[Unit]
Description=Keep X11 DPMS Disabled, Prevent Monitor Auto Off
After=graphical-session.target

[Service]
Type=simple
User=guan
Environment="DISPLAY=:0"
Environment="XAUTHORITY=/home/guan/.Xauthority"
ExecStart=/bin/bash -c 'while true; do xset -dpms; xset s off; sleep 3; done'
Restart=always
RestartSec=5

[Install]
WantedBy=graphical-session.target
EOF

systemctl daemon-reload
systemctl enable dpms-keep-active.service
systemctl start dpms-keep-active.service

echo "============================================="
echo "服务安装完成！"
echo "查看状态: systemctl status dpms-keep-active.service"
echo "停止: sudo systemctl stop dpms-keep-active.service"
echo "============================================="

# 卸载方式

# sudo systemctl stop dpms-keep-active.service
# sudo systemctl disable dpms-keep-active.service
# sudo rm /etc/systemd/system/dpms-keep-active.service
# sudo systemctl daemon-reload