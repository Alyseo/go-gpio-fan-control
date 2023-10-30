if [ -f "/lib/systemd/system/gpio-fan-control.service" ]; then
    systemctl daemon-reload
    systemctl enable gpio-fan-control.service
    systemctl start gpio-fan-control.service
fi