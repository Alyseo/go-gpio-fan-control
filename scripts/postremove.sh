if [ -f "/lib/systemd/system/gpio-fan-control.service" ]; then
  systemctl stop gpio-fan-control.service
  systemctl disable gpio-fan-control.service
fi