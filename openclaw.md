# OpenClaw Gateway — VPS Setup Guide

> Setup ini berdasarkan konfigurasi aktif di VPS `andromedahub.my.id`
> OpenClaw berjalan **langsung di host VPS** (bukan Docker), port `18789`

---

## 📋 Daftar Isi

1. [Instalasi OpenClaw](#1-instalasi-openclaw)
2. [Konfigurasi Telegram Bot](#2-konfigurasi-telegram-bot)
3. [Menjalankan Gateway](#3-menjalankan-gateway)
4. [Auto-Start dengan Systemd](#4-auto-start-dengan-systemd)
5. [Cek Status](#5-cek-status)
6. [Troubleshooting](#6-troubleshooting)

---

## 1. Instalasi OpenClaw

```bash
# Install Node.js 20 (jika belum ada)
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt install -y nodejs

# Install OpenClaw CLI secara global
npm install -g openclaw

# Verifikasi instalasi
openclaw --version
```

---

## 2. Konfigurasi Telegram Bot

### 2a. Buat Bot Telegram
1. Buka Telegram → cari `@BotFather`
2. Ketik `/newbot`
3. Ikuti instruksi → copy **Bot Token** yang diberikan
4. Format token: `123456789:AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`

### 2b. Daftarkan Bot ke OpenClaw

```bash
openclaw channels add --channel telegram --token YOUR_BOT_TOKEN
```

Contoh:
```bash
openclaw channels add --channel telegram --token 8637447834:AAGlYRTX5XlDUp9J_DNLFBB1aAGSfzQQKQY
```

Output sukses:
```
Added Telegram account "default".
```

### 2c. Verifikasi Channel

```bash
openclaw channels list
```

---

## 3. Menjalankan Gateway

### Jalankan manual (foreground, untuk testing):
```bash
openclaw gateway run
```

### Jalankan di background (sementara):
```bash
nohup openclaw gateway run > /var/log/openclaw-gateway.log 2>&1 &
```

### Cek gateway berjalan:
```bash
ps aux | grep openclaw-gateway | grep -v grep
ss -tlnp | grep 18789
```

---

## 4. Auto-Start dengan Systemd

> Supaya OpenClaw otomatis nyala setiap VPS reboot

### 4a. Buat service file

```bash
nano /etc/systemd/system/openclaw-gateway.service
```

Isi dengan:
```ini
[Unit]
Description=OpenClaw Gateway
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/openclaw gateway run
Restart=always
RestartSec=5
StandardOutput=append:/var/log/openclaw-gateway.log
StandardError=append:/var/log/openclaw-gateway.log

[Install]
WantedBy=multi-user.target
```

### 4b. Aktifkan service

```bash
# Reload systemd
systemctl daemon-reload

# Enable agar auto-start saat reboot
systemctl enable openclaw-gateway

# Start sekarang
systemctl start openclaw-gateway

# Cek status
systemctl status openclaw-gateway
```

### 4c. Perintah systemd berguna

```bash
systemctl start openclaw-gateway    # Start
systemctl stop openclaw-gateway     # Stop
systemctl restart openclaw-gateway  # Restart
systemctl status openclaw-gateway   # Cek status
journalctl -u openclaw-gateway -f   # Lihat logs live
```

---

## 5. Cek Status

### Cek semua sekaligus:
```bash
echo "=== PROCESS ===" && ps aux | grep openclaw-gateway | grep -v grep
echo "=== PORT ===" && ss -tlnp | grep 18789
echo "=== CHANNEL ===" && openclaw channels status --probe
```

### Cek logs gateway:
```bash
tail -f /var/log/openclaw-gateway.log
```

### Test kirim pesan Telegram:
> **Penting:** User harus `/start` ke bot dulu sebelum bisa dikirim pesan!

```bash
openclaw message send --channel telegram --target @username_kamu --message "Test dari OpenClaw!"
```

---

## 6. Troubleshooting

### ❌ Gateway tidak bisa start (port sudah dipakai)
```bash
# Cek PID yang pakai port 18789
ss -tlnp | grep 18789

# Kill process lama
kill <PID>

# Start ulang
systemctl start openclaw-gateway
```

### ❌ Gateway timeout / tidak reachable
```bash
# Cek apakah process jalan
ps aux | grep openclaw-gateway | grep -v grep

# Kalau tidak ada, start ulang
systemctl start openclaw-gateway
```

### ❌ Telegram: chat not found
```bash
# Penyebab: user belum /start ke bot
# Fix: buka Telegram → cari @nama_bot_kamu → klik Start
# Lalu coba kirim pesan lagi
```

### ❌ Cek doctor untuk diagnosis lengkap
```bash
openclaw doctor
```

---

## 📁 File Penting

| File | Keterangan |
|------|------------|
| `~/.openclaw/openclaw.json` | Config utama OpenClaw |
| `~/.openclaw/agents/main/` | Data agent & sessions |
| `/var/log/openclaw-gateway.log` | Log gateway |
| `/etc/systemd/system/openclaw-gateway.service` | Systemd service |

---

## 🌐 Integrasi Nginx

OpenClaw UI diakses via `https://claw.andromedahub.my.id` dengan basic auth.
Nginx memproxy request ke `host.docker.internal:18789`.

Config nginx yang relevan:
```nginx
server {
    listen 443 ssl;
    server_name claw.andromedahub.my.id;

    auth_basic "OpenClaw";
    auth_basic_user_file /etc/nginx/.htpasswd;

    location / {
        proxy_pass http://host.docker.internal:18789/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 120s;
    }
}
```

Untuk update password basic auth:
```bash
apt install apache2-utils -y
htpasswd -c /etc/nginx/.htpasswd admin
docker exec nginx nginx -s reload
```

---

*Dibuat berdasarkan setup aktif di VPS andromedahub.my.id — April 2026*