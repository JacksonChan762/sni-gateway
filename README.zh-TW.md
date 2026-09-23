# SNI Gateway — sing-box 供應商與自訂 VPN 出口

[English](README.md)

這個開發分支使用 Go SNI 入口。供應商與自訂 VPN 出口都由 sing-box 直接連線。管理介面及 API 預設位於 `127.0.0.1:8080`。後端直接處理 80 和 443 連接埠：80 將有規則的網域轉至 HTTPS；443 根據 TLS SNI 選擇 VPN 出口，原始 TLS 連線不會在閘道解密。沒有符合規則的 SNI 會被拒絕。

這是遷移分支，並非 v1.1 穩定版；[v1.1 發行說明](docs/releases/v1.1.md)仍描述 Nginx/Gluetun 架構。

## 需求

- Docker 與 Docker Compose v2
- 主機的 80、443 連接埠可用
- 供應商出口只需依畫面輸入該供應商必要的個人資料：OpenVPN 帳密（少數還需用戶端憑證與金鑰），或 WireGuard 私鑰及必要時的通道位址／預共享金鑰。自訂出口仍需完整的 WireGuard 設定或 OpenVPN `.ovpn`。

## 安裝

```bash
./sni-gateway.sh install
```

首次執行會建立 `.env` 並產生隨機管理員密碼。到 `http://127.0.0.1:8080` 登入，密碼在 `.env` 的 `ADMIN_PASSWORD`。請妥善保管這個檔案。安裝程式會建置後端與介面，並下載官方 sing-box 映像。新服務健康後，會移除舊的 Nginx Compose 服務並停止舊 relay 容器；PostgreSQL 資料會保留。

現有部署切換分支前，請先備份資料庫。從舊 Nginx/Gluetun 版本首次遷移時，先停止舊 Compose 服務、切換到此分支，再執行 `./sni-gateway.sh install`，以免 80/443 連接埠衝突。之後可使用[更新說明](docs/UPDATES.md)中的同一支腳本更新。

## 使用方式

1. 到**出口**選內建供應商、國家或城市，以及協定。國家可用來縮小城市選單；若選了城市，連線時只以城市篩選端點。OpenVPN 填供應商專用帳密；畫面會對需要憑證的供應商顯示額外欄位。WireGuard 填私鑰及畫面列出的必要資料。若使用自己的 VPN 伺服器，選 **Custom configuration** 並匯入完整 `.conf` 或 `.ovpn`。出口啟動時會透過 VPN 查詢公網 IP；「Running」代表檢查成功，「Unhealthy」代表連線未通過檢查。可查看「Logs」並按「Check status」重試。
2. 建立精確或尾碼網域規則並選出口。若規則指向未轉換的舊出口，需先編輯出口才能發布。
3. 到**發布**頁驗證並發布規則。新連線會立即使用已發布的規則表；回復功能會還原上一版。訂閱同步也能自動發布變更。
4. 將網域指向閘道主機並測試 TLS。目的網域的 DNS 查詢經由選定的 VPN；sing-box 將該出口的 SOCKS 流量只路由到 VPN 端點。

WireGuard 的環境變數 YAML 與 OpenVPN 的 YAML 匯出內容都是本程式的匯入／匯出格式，並非可獨立執行的 Docker Compose 檔案。每個出口的 sing-box SOCKS 1080 僅在 Docker 私有 `vpn-net` 網路提供，不對主機公開。

API 請參考[介面文件](docs/API.md)，包含 WireGuard 與 OpenVPN 請求範例。自訂 OpenVPN 伺服器若使用網域名稱，建立隧道前會先由本機 DNS 解析；目的網站的查詢仍經 VPN。

## 維運

```bash
./sni-gateway.sh update --check
./sni-gateway.sh update --apply
./scripts/stop.sh
cd backend && go test ./...
cd frontend && npm run build
```

`update --apply` 會備份 PostgreSQL、快轉追蹤分支、重建映像並等待服務健康。現有 VPN 出口不會自動重啟；需要更新執行映像時，請在介面逐一重新啟動。後端需要 Docker socket 以管理出口。`.env` 與備份包含私密資訊，請妥善保存。

介面與 sing-box 供應商模式使用 [Gluetun 伺服器資料](https://github.com/qdm12/gluetun-servers)，啟動時及每日檢查更新。國家用於篩選城市；選定城市後會忽略國家，城市選「Any」時則依國家挑選。每次連線會實測最多四個候選端點，選擇成功連線且測得延遲最低的端點；WireGuard 與 OpenVPN 使用相同規則。目錄 IP 是 VPN 端點，畫面公網 IP 則是連線後實測取得。OpenVPN 的公開 CA 與 TLS 範本取自 Gluetun 供應商定義；新出口不會執行 Gluetun 容器。發行版可用 `python3 scripts/update_provider_catalog.py /path/to/gluetun-servers` 更新內建節點快照；供應商定義變更時，以 `python3 scripts/update_openvpn_templates.py /path/to/gluetun` 更新 OpenVPN 範本並重新檢查 sing-box 設定。授權文件見 `backend/internal/catalog/data/LICENSE.gluetun-servers` 與 `backend/internal/provider/data/LICENSE.gluetun`。

選擇依建立 VPN 容器到成功取得公網 IP 的耗時排序；若該端點在正式連線時失敗，會改試其他測試成功的端點。候選超過四個時只抽測四個，結果不代表全目錄的絕對最快。每個端點最多等待 40 秒，整次連線最多約五分鐘。**Check status** 只檢查目前端點；要重新測速並選擇端點，請按 **Restart**。更新程式後，原本運行中的出口也需按 **Restart** 才會採用新規則。

目前僅支援帶有 SNI 的 TCP HTTPS，不轉送 QUIC/HTTP3 或無 SNI 的 TLS。完整整合測試需要 Docker；單元測試與 sing-box 設定檢查不會建立真正的 VPN 通道。
