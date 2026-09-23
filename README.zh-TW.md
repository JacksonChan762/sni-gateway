# SNI Gateway — sing-box 遷移版

[English](README.md)

這個開發分支以 Go SNI 入口與每個 WireGuard 或 OpenVPN 出口一個 sing-box 容器，取代 Nginx、Gluetun 和 relay。管理介面及 API 預設位於 `127.0.0.1:8080`。後端直接處理 80 和 443 連接埠：80 將有規則的網域轉至 HTTPS；443 根據 TLS SNI 選擇 VPN 出口，原始 TLS 連線不會在閘道解密。沒有符合規則的 SNI 會被拒絕。

這是遷移分支，並非 v1.1 穩定版；[v1.1 發行說明](docs/releases/v1.1.md)仍描述 Nginx/Gluetun 架構。

## 需求

- Docker 與 Docker Compose v2
- 主機的 80、443 連接埠可用
- 每個出口都有**完整的 VPN 用戶端設定**：WireGuard 介面與節點資料，或包含內嵌 CA、必要時含用戶端憑證與金鑰的 OpenVPN `.ovpn`；需要帳密時另行輸入。

單靠 VPN 供應商帳號密碼與國家選項，仍無法取得 VPN 所需的用戶端私鑰、通道位址、CA 憑證或供應商專用認證資料。舊出口會保留在資料庫；需匯入完整的 WireGuard 或 OpenVPN 設定，才能啟動或發布。OpenVPN 設定中的外部憑證／金鑰路徑須改為內嵌 `<ca>`、`<cert>`、`<key>` 與 TLS 金鑰區塊；不支援的指令會在匯入時明確報錯。

## 安裝

```bash
./sni-gateway.sh install
```

首次執行會建立 `.env` 並產生隨機管理員密碼。到 `http://127.0.0.1:8080` 登入，密碼在 `.env` 的 `ADMIN_PASSWORD`。請妥善保管這個檔案。安裝程式會建置後端與介面，並下載固定版本的官方 sing-box 映像。新服務健康後，會移除舊的 Nginx Compose 服務並停止舊 Gluetun/relay 容器；PostgreSQL 資料會保留。

現有部署切換分支前，請先備份資料庫。從舊 Nginx/Gluetun 版本首次遷移時，先停止舊 Compose 服務、切換到此分支，再執行 `./sni-gateway.sh install`，以免 80/443 連接埠衝突。之後可使用[更新說明](docs/UPDATES.md)中的同一支腳本更新。

## 使用方式

1. 到**出口**新增設定，匯入供應商的 WireGuard `.conf` 或 OpenVPN `.ovpn`，也可手動輸入 WireGuard 欄位。可用供應商目錄搜尋國家與節點；選取後會填入伺服器 IP 與 WireGuard 公鑰，或替換 `.ovpn` 的遠端位址。WireGuard 介面位址需含 CIDR，例如 `10.2.0.2/32`；端點需為 IP。OpenVPN `.ovpn` 需包含內嵌 CA 憑證，供應商要求帳密時請另行輸入。出口啟動時會透過 VPN 查詢公網 IP；「Running」代表檢查成功，「Unhealthy」代表容器已啟動但 VPN 檢查失敗。可查看「Logs」並按「Check status」重試檢查。
2. 建立精確或尾碼網域規則並選出口。若規則指向舊出口，必須先補齊 WireGuard 或 OpenVPN 設定才能發布。
3. 到**發布**頁驗證並發布規則。新連線會立即使用已發布的規則表；回復功能會還原上一版。訂閱同步也能自動發布變更。
4. 將網域指向閘道主機並測試 TLS。目的網域的 DNS 查詢由選定的 sing-box 出口處理，沒有直接連線的備援路徑。

WireGuard 的 `services.singbox.environment` YAML 與 OpenVPN 的 YAML 匯出內容都是本程式的匯入／匯出格式，並非可獨立執行的 Docker Compose 檔案。動態 sing-box 容器的 SOCKS 1080 僅在 Docker 私有 `vpn-net` 網路提供，不對主機公開。

API 請參考[介面文件](docs/API.md)，包含 WireGuard 與 OpenVPN 請求範例。OpenVPN 伺服器若使用網域名稱，建立隧道前會先由本機 DNS 解析；目的網站的查詢仍經 VPN。

## 維運

```bash
./sni-gateway.sh update --check
./sni-gateway.sh update --apply
./scripts/stop.sh
cd backend && go test ./...
cd frontend && npm run build
```

`update --apply` 會備份 PostgreSQL、快轉追蹤分支、重建映像並等待服務健康。現有的動態 sing-box 出口不會自動重啟；需要更新其執行映像時，請在介面逐一重新啟動。後端需要 Docker socket 以管理出口。`.env` 與備份包含私密資訊，請妥善保存。

內建供應商目錄是 [Gluetun 伺服器資料](https://github.com/qdm12/gluetun-servers)的固定快照（23 家供應商、27,187 個節點；來源提交 `0c7381f`）。資料採 MIT 授權，見 `backend/internal/catalog/data/LICENSE.gluetun-servers`。選節點只能填入連線端點，仍需供應商提供的 WireGuard 用戶端設定或 OpenVPN 設定檔。更新目錄時，先複製上游儲存庫，再執行 `python3 scripts/update_provider_catalog.py /path/to/gluetun-servers` 並檢查變更。已選節點會存入出口設定，不會隨目錄更新而自動變更。

目前僅支援帶有 SNI 的 TCP HTTPS，不轉送 QUIC/HTTP3 或無 SNI 的 TLS。完整整合測試需要 Docker；單元測試與 sing-box 設定檢查不會建立真正的 VPN 通道。
