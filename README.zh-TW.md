# SNI Gateway

[English](README.md) | [繁體中文](README.zh-TW.md)

SNI Gateway 是一個透過網頁介面管理的 TLS SNI 流量路由閘道器。它在邊緣節點接收 HTTP/HTTPS 流量，根據資料庫中的規則比對網域，並將符合條件的 HTTPS 連線透過指定的 VPN 出口轉發。

## 目錄

- [運作原理](#運作原理)
- [系統需求](#系統需求)
- [快速開始](#快速開始)
- [設定說明](#設定說明)
- [基本使用流程](#基本使用流程)
- [訂閱清單格式](#訂閱清單格式)
- [API 認證](#api-認證)
- [API 範例](#api-範例)
  - [建立 WireGuard 出口](#建立-wireguard-出口)
  - [新增規則](#新增規則)
  - [新增訂閱](#新增訂閱)
  - [發布 Nginx 設定](#發布-nginx-設定)
  - [測試路由](#測試路由)
- [運維注意事項](#運維注意事項)
- [開發](#開發)
- [文件](#文件)

## 運作原理

```
客戶端
  │
  ├── 連接埠 80  → nginx（HTTP）→ 將符合條件的主機重新導向至 HTTPS
  │
  └── 連接埠 443 → nginx（stream，ssl_preread）
                    │
                    ├── SNI 符合規則 → relay sidecar（在 gluetun VPN 網路命名空間內）
                    │                   └── 透過 VPN proxy_pass $sni_host:443
                    │
                    └── SNI 不符合規則 → 預設後端（不走 VPN）
```

技術棧：

- **Nginx** — 邊緣 HTTP 重新導向與 TLS SNI TCP 流量路由。
- **Gluetun** — 每個 VPN 出口一個容器，建立獨立的 VPN 網路命名空間。
- **Relay sidecar** — 每個出口一個 nginx 容器，運行於 Gluetun 網路命名空間內，透過 VPN 轉發 SNI 流量。
- **Go/Fiber 後端** — REST API，管理出口、規則與訂閱；生成並套用 nginx 設定；建立/移除出口的 Docker 容器。
- **React 管理介面** — 由後端提供服務，使用 JWT Token 與 API 溝通。
- **PostgreSQL** — 儲存出口、規則、訂閱及發布歷史記錄。

規則先儲存至資料庫，只有在發布並套用生成的設定後才會在 nginx 中生效。

## 系統需求

- Docker 26+
- Docker Compose v2
- 主機的 80 和 443 連接埠未被佔用

在首次啟動前先生成強度足夠的 `JWT_SECRET`：

```bash
openssl rand -hex 32
```

## 快速開始

建立 `.env` 檔案：

```bash
cp .env.example .env
# 編輯 .env，設定 JWT_SECRET 和 ADMIN_PASSWORD
```

或直接建立：

```bash
cat > .env <<'EOF'
JWT_SECRET=請替換為長且隨機的密鑰
ADMIN_USERNAME=admin
ADMIN_PASSWORD=請替換此密碼
EOF
```

啟動服務：

```bash
docker compose up --build -d
```

檢查健康狀態：

```bash
curl http://127.0.0.1:8080/health
```

開啟管理介面：

```
http://127.0.0.1:8080
```

預設連接埠：

| 連接埠 | 用途 |
| --- | --- |
| `8080` | 管理介面與 API |
| `80` | 邊緣 HTTP（將符合條件的主機重新導向至 HTTPS） |
| `443` | 邊緣 HTTPS / SNI 路由 |

## 設定說明

後端的環境變數：

| 變數名稱 | 預設值 | 說明 |
| --- | --- | --- |
| `JWT_SECRET` | `change-me-in-production` | 用於簽署管理員 JWT。務必在 `.env` 中設定。 |
| `ADMIN_USERNAME` | `admin` | 管理員登入帳號。 |
| `ADMIN_PASSWORD` | `admin` | 管理員登入密碼。務必在 `.env` 中設定。 |
| `DATABASE_URL` | 由 compose 管理 | PostgreSQL 連線字串。 |
| `DOCKER_SOCKET` | `/var/run/docker.sock` | 用於建立出口容器的 Docker socket。 |
| `NGINX_CONFIG_DIR` | `/etc/nginx/conf.d` | 生成的 nginx 設定寫入的共享目錄。 |
| `EXIT_RESTART_CRON` | `@every 12h` | 重新啟動所有啟用出口的排程；設為 `disabled` 可停用。 |

後端需要 Docker socket 存取權限，因為它在出口啟動或刪除時會建立和移除 `gluetun-*` 與 `relay-*` 容器。
預設每 12 小時重新啟動所有啟用的出口。

## 基本使用流程

1. 登入管理介面（`http://127.0.0.1:8080`）。
2. 建立 VPN 出口（透過 Gluetun 支援 WireGuard 或 OpenVPN）。
3. 啟動出口，確認其狀態與公網 IP。
4. 建立手動路由規則或新增訂閱。
5. 前往「發布」→ 預覽 → 套用，寫入並重新載入 nginx 設定。
6. 透過閘道器測試目標網域。

規則和訂閱先儲存在資料庫中，只有套用生成的設定後才會在 nginx 中生效。

## 訂閱清單格式

訂閱功能會下載遠端網域清單檔案，並將每一行轉換為路由規則。解析器支援以下格式：

```
# 以 # 開頭的行為注釋，會被忽略

# 純網域 → 精確比對規則
example.com

# 點前綴 → 後綴比對規則（比對 example.com 及所有子網域）
+.example.com

# Clash/Surge/Meta 格式，保留精確／後綴比對類型
DOMAIN,example.com
DOMAIN-SUFFIX,example.com
```

訂閱規則的優先級為 `50`，手動規則預設優先級為 `100`，手動規則優先被評估。

### 訂閱同步行為

- `DOMAIN-SUFFIX,example.com` 保留後綴匹配，可接受第三欄策略名稱。
- 網域統一轉為小寫並移除重複規則；使用 ASCII 或 punycode 網域。不支援的規則類型與無效網域會略過。
- 下載逾時為 30 秒，超過 10 MiB 的檔案會被拒絕，不會套用截斷內容。
- 規則採分批寫入，並在同一資料庫交易中替換；資料庫寫入失敗會保留原有規則。交易提交後若 nginx 發布失敗，會另外回報錯誤。
- 同步失敗後仍保留排程，於下次排程重試。

## API 認證

除 `/api/auth/login` 外，所有 `/api/*` 路由都需要 Bearer Token。

登入並儲存 Token：

```bash
TOKEN="$(
  curl -s -X POST http://127.0.0.1:8080/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"請替換此密碼"}' |
  sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
)"
```

使用 Token：

```bash
curl http://127.0.0.1:8080/api/stats \
  -H "Authorization: Bearer $TOKEN"
```

Token 有效期為 24 小時。管理介面會在每次請求前檢查有效期，Token 過期時會自動跳轉至登入頁面。

## API 範例

### 建立 WireGuard 出口

```bash
curl -X POST http://127.0.0.1:8080/api/exits \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "us",
    "enabled": true,
    "config_yaml": "enabled: true\nservices:\n  gluetun:\n    image: qmcgaw/gluetun:latest\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - SERVER_CITIES=\n      - WIREGUARD_PRIVATE_KEY=YOUR_PRIVATE_KEY\n      - WIREGUARD_ADDRESSES=10.14.0.2/16\n"
  }'
```

出口設定以標準化格式儲存。管理介面編輯 `services.gluetun.environment` 的值，並保留額外的 Gluetun 環境變數，包含 wg0 格式的自訂 WireGuard 供應商設定。

### 新增規則

```bash
curl -X POST http://127.0.0.1:8080/api/rules \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "example.com",
    "type": "suffix",
    "exit_id": 1,
    "priority": 100,
    "enabled": true
  }'
```

規則類型：

- `exact` — 精確比對單一主機名稱。
- `suffix` — 比對該網域及其所有子網域。
- `auto` 或省略 — `+.` 開頭的 pattern 會建立為 suffix 規則，其他 pattern 會建立為 exact 規則。

優先級數值越高，越先被評估。手動規則預設為 `100`，訂閱規則為 `50`。

### 新增訂閱

```bash
curl -X POST http://127.0.0.1:8080/api/subscriptions \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "AI 網站",
    "url": "https://example.com/ai.list",
    "exit_id": 1,
    "enabled": true,
    "interval_minutes": 360
  }'
```

後端會立即同步清單，並按 `interval_minutes` 定期重新同步。

### 發布 Nginx 設定

預覽：

```bash
curl -X POST http://127.0.0.1:8080/api/publish/preview \
  -H "Authorization: Bearer $TOKEN"
```

套用：

```bash
curl -X POST http://127.0.0.1:8080/api/publish/apply \
  -H "Authorization: Bearer $TOKEN"
```

回滾：

```bash
curl -X POST http://127.0.0.1:8080/api/publish/rollback \
  -H "Authorization: Bearer $TOKEN"
```

### 測試路由

若 DNS 已指向此伺服器：

```bash
curl -I http://example.com/
curl -vk https://example.com/
```

若 DNS 尚未指向伺服器，使用 `--resolve`：

```bash
curl -I --resolve example.com:80:你的伺服器IP http://example.com/
curl -vk --resolve example.com:443:你的伺服器IP https://example.com/
```

原始 TLS SNI 測試：

```bash
openssl s_client -connect 你的伺服器IP:443 -servername example.com
```

## 運維注意事項

- 變更出口、規則或訂閱不會自動發布 nginx 設定。路由變更後請執行「發布 → 套用」。
- 執行中的出口在名稱或 runtime 設定變更後會自動協調；已停止的出口會在下次啟動時重建。
- 預設每 12 小時重新啟動所有啟用的出口。
- 當有作用中的規則或訂閱仍指向某個出口時，無法刪除該出口，請先停用或刪除這些相依項目。
- 停用或刪除訂閱會移除其生成的規則，並自動重新載入 nginx 設定。
- Relay 映像檔（`sni-gateway-relay:latest`）由 compose 建置。出口建立時需要此映像檔存在，因此首次啟動請務必執行 `docker compose up --build`。
- 連接埠 `443` 是真正的 SNI 轉發路徑；連接埠 `80` 僅將符合條件的主機重新導向至 HTTPS。
- 訂閱規則的優先級為 `50`，手動規則預設為 `100`，手動規則優先被評估。

## 開發

後端：

```bash
cd backend
go test ./...
go build ./...
```

前端（需要 Node.js 16，Vite 4）：

```bash
cd frontend
npm install
npm run build
```

前端建置結果會輸出至 `backend/public/`，由後端在根路徑提供服務。

## 文件

- API 參考：[docs/API.md](docs/API.md)

## 自動更新

- 應用程式：支援選用的每日自動更新、資料庫備份與服務健康檢查。設定及復原步驟請參閱 [更新指南（英文）](docs/UPDATES.md)。
- 儀表板：每 15 秒重新整理，可暫停或手動更新，並提供初次設定指引。
- 訂閱：啟用的清單依設定排程同步，成功後會自動發布路由設定（包含尚未發布的手動變更）。
- Docker 會直接從原始碼建置前端，更新時不需在主機安裝 Node.js。

```bash
bash scripts/update.sh --check
bash scripts/update.sh --apply
```

## 匹配效能與關閉操作

- 使用 nginx 原生 hostname map，在發布前解決精確／後綴規則的優先級衝突，移除逐條正則匹配與重複鍵。優先級較高者勝出；相同優先級採用較小的規則 ID。網域不分大小寫，HTTP Host 的 port 不影響匹配。
- Publish → Test domain routing 可預覽已儲存規則的匹配結果，包含尚未發布的變更；此功能不是實際連線測試。
- Exits → Stop All 同時停止各出口的 VPN 與 relay，逐項顯示失敗原因，保留規則與訂閱。可在介面重新啟動已啟用的出口。停止的出口不會被定時重啟重新開啟，其路由亦不會自動轉往另一出口。
- 完整停止服務與動態建立的出口：`bash scripts/stop.sh`。此指令先停止後端，再停止資料庫記錄的 VPN／relay，最後停止 Compose 服務，保留設定與資料卷。
- 恢復管理介面：`docker compose up -d`，再手動啟動需要的出口。單獨重啟後端會保留 VPN 運行；直接使用 `docker compose stop` 不會處理動態出口。
- 驗證：後端 `go test -race ./...`；前端 `npm ci && npm run build`；腳本 `python3 -m unittest discover -s scripts/tests`。nginx 整合測試需要 nginx 與 `/usr/lib/nginx/modules/ngx_stream_module.so`，缺少時會標示略過。
