# SNI 多 VPN 分流服務開發文件

## 1. 專案定位

本專案是一套以 **SNI（Server Name Indication）** 為核心的多出口流量分流平台。

系統接收外部 TLS/HTTP 連線後，依照 **SNI 網域名稱** 或 **HTTP Host**，將流量導向不同的 VPN 出口，讓不同網站或網域群組可使用不同國家或地區的出口 IP。

本系統支援兩種規則來源：

1. **遠端 `.list` 訂閱規則**
2. **WebUI 手動建立規則**

本系統同時提供 **WebUI 出口管理功能**，可直接在介面中：

- 新增 Gluetun 出口容器
- 刪除 Gluetun 出口容器
- 啟動 Gluetun 出口容器
- 停止 Gluetun 出口容器
- 重啟 Gluetun 出口容器
- 查看出口狀態、外網 IP、地區、健康狀態

---

## 2. 專案目標

### 2.1 核心目標

建立一套具備以下能力的服務：

- 依照 SNI 將 TLS 連線分流到不同 VPN 出口
- 依照 Host 將 HTTP 流量分流到不同 VPN 出口
- 支援多個 VPN 出口並行運作
- 支援遠端規則訂閱與自動更新
- 支援 WebUI 規則管理
- 支援 WebUI 動態管理 Gluetun 容器生命週期
- 支援設定檔自動產生與熱更新

### 2.2 產品目標

讓使用者可以透過瀏覽器管理整套系統，包括：

- 新增規則
- 新增訂閱
- 綁定出口
- 建立新的 VPN 出口
- 啟停 VPN 出口
- 套用設定
- 觀察流量命中結果

---

## 3. 使用情境

### 3.1 基本情境

| 網域規則 | 出口 |
|---|---|
| `chatgpt.com` | us |
| `+.openai.com` | us |
| `+.claude.ai` | jp |
| `+.anthropic.com` | jp |
| `netflix.com` | sg |
| 其他流量 | default |

### 3.2 訂閱情境

使用者加入一個遠端規則清單：

`https://github.com/MetaCubeX/meta-rules-dat/raw/refs/heads/meta/geo/geosite/category-ai-!cn.list`

並指定這份清單的所有規則都走 `us` 出口。

系統定時下載該規則清單，解析其中的網域項目，編譯成內部規則，再下發到資料平面。

### 3.3 WebUI 出口管理情境

使用者在 WebUI 中新增一個新出口：

- 名稱：`jp`
- 類型：`gluetun`
- 協定：`wireguard`
- 國家：`Japan`
- 憑證：`WireGuard Private Key`

系統收到請求後：

1. 建立對應的 Gluetun 容器設定
2. 建立對應的 relay 容器設定
3. 啟動容器
4. 檢查 VPN 連線狀態
5. 將新出口加入規則可選清單

---

## 4. 系統架構

```text
Client
  ↓
Edge Gateway (Nginx)
  ↓ 讀取 SNI / Host
Rule Engine
  ↓
Exit Selector
  ├─ Exit: us
  │   ├─ gluetun-us
  │   └─ relay-us
  ├─ Exit: jp
  │   ├─ gluetun-jp
  │   └─ relay-jp
  └─ Exit: sg
      ├─ gluetun-sg
      └─ relay-sg
```

---

## 5. 元件設計

## 5.1 Edge Gateway

### 職責

- 監聽 `80` 與 `443`
- 在 `443` 連線中讀取 TLS ClientHello 的 SNI
- 在 `80` 流量中讀取 HTTP Host
- 依照規則將流量轉送到對應 relay

### 建議技術

- Nginx
- Nginx stream module
- Nginx http proxy

### 設計原則

- `443` 使用 TLS passthrough，不終止 TLS
- `80` 使用 Host-based proxy
- Edge 僅負責路由，不負責規則解析與訂閱抓取

---

## 5.2 Rule Engine

### 職責

- 載入所有規則來源
- 規則正規化
- 處理優先序
- 編譯成 Nginx 可用的映射配置

### 規則來源

1. 手動規則
2. 訂閱規則
3. 預設出口

### 支援格式

```text
chatgpt.com
+.openai.com
+.claude.ai
```

### 規則類型

- `exact`：精確匹配
- `suffix`：子網域/後綴匹配

### 規則優先序

```text
1. WebUI 精確規則
2. WebUI 後綴規則
3. 訂閱精確規則
4. 訂閱後綴規則
5. Default 出口
```

---

## 5.3 Subscription Manager

### 職責

- 新增訂閱來源
- 修改訂閱來源
- 刪除訂閱來源
- 定時抓取 `.list` 檔案
- 解析規則
- 更新本地快取
- 觸發規則重編譯

### 支援能力

- HTTP/HTTPS 拉取
- 定時更新
- 手動更新
- 啟用/停用訂閱
- 綁定預設出口

### 訂閱資料範例

```json
{
  "name": "AI Sites",
  "url": "https://example.com/ai.list",
  "exit": "us",
  "enabled": true,
  "interval_minutes": 360
}
```

### `.list` 解析規則

- 忽略空行
- 忽略註解行
- `example.com` 視為精確規則
- `+.example.com` 視為後綴規則

---

## 5.4 Exit Manager

### 職責

管理所有 VPN 出口節點。

每個出口節點由以下兩個容器組成：

1. **Gluetun 容器**：建立 VPN 隧道
2. **Relay 容器**：共用 Gluetun 網路命名空間，作為實際轉送層

### 出口資料模型

| 欄位 | 說明 |
|---|---|
| name | 出口名稱，例如 `us` |
| runtime | 固定為 `gluetun` |
| protocol | `wireguard` 或 `openvpn` |
| country | 國家 |
| city | 城市，可選 |
| provider | VPN 供應商 |
| status | running / stopped / unhealthy |
| public_ip | 目前出口 IP |
| enabled | 是否可被規則使用 |

### WebUI 必須支援的操作

- 新增出口
- 編輯出口
- 刪除出口
- 啟動出口
- 停止出口
- 重啟出口
- 測試出口
- 查詢出口外網 IP
- 查看容器狀態
- 查看健康狀態

### 容器生命週期管理需求

WebUI 需要能直接操作容器執行狀態，因此後端必須提供下列能力：

- 建立 Gluetun 容器
- 建立 relay 容器
- 啟動容器
- 停止容器
- 刪除容器
- 重建容器
- 查詢容器 logs
- 查詢容器 health status

### 建議實作方式

後端透過 Docker Engine API 或 Docker SDK 管理容器。

### 一個出口的標準組成

- `gluetun-us`
- `relay-us`

### 命名規則

- Gluetun 容器：`gluetun-{exit_name}`
- Relay 容器：`relay-{exit_name}`

---

## 5.5 Config Generator

### 職責

- 將規則引擎輸出的映射表轉換為 Nginx 設定
- 驗證設定正確性
- 寫入設定檔
- 觸發 Nginx reload

### 產出內容

1. `stream` 區塊：處理 `443`
2. `http` 區塊：處理 `80`
3. `map` 對應表：網域 → backend

### 產生範例

```nginx
map $ssl_preread_server_name $tls_backend {
    chatgpt.com backend_us;
    ~(^|\.)openai\.com$ backend_us;
    ~(^|\.)claude\.ai$ backend_jp;
    default backend_default;
}
```

### 發布流程

1. 產生暫存設定檔
2. 執行 `nginx -t`
3. 驗證成功後覆蓋正式設定檔
4. 執行 `nginx -s reload`
5. 保留前一版設定以支援回滾

---

## 5.6 Relay Layer

### 職責

每個 relay 容器負責：

- 接收來自 edge gateway 的 TCP/HTTP 轉送流量
- 將流量轉送到目標站點
- 因共用 Gluetun 網路命名空間而使用指定 VPN 出口

### 設計原則

- relay 不直接對外暴露公開入口
- relay 僅接受 edge gateway 轉送
- relay 設定保持輕量

---

## 5.7 WebUI

### 功能模組

#### Dashboard

顯示：

- 總規則數
- 啟用中的訂閱數
- 啟用中的出口數
- 出口健康狀態
- 最近配置發布時間
- 最近訂閱更新時間

#### 規則管理

功能：

- 新增規則
- 編輯規則
- 刪除規則
- 啟用/停用規則
- 指定出口
- 調整優先序
- 搜尋規則
- 預覽命中結果

#### 訂閱管理

功能：

- 新增訂閱 URL
- 編輯訂閱
- 刪除訂閱
- 啟用/停用訂閱
- 指定出口
- 手動更新
- 查看上次更新時間
- 查看解析成功筆數
- 查看更新錯誤訊息

#### 出口管理

功能：

- 新增 Gluetun 出口
- 編輯出口參數
- 刪除出口
- 啟動出口
- 停止出口
- 重啟出口
- 測試出口
- 查看外網 IP
- 查看容器 logs
- 查看健康狀態

#### 發布管理

功能：

- 預覽即將產生的配置
- 執行配置測試
- 套用配置
- 回滾到上一版本
- 顯示發布紀錄

#### 日誌與觀測

功能：

- 查詢命中日誌
- 查詢出口使用統計
- 查詢錯誤日誌
- 查詢訂閱更新日誌

---

## 6. VPN Exit 技術選型

### 6.1 選型結論

本系統採用：

- **Runtime：Gluetun**
- **VPN Protocol：WireGuard 優先**
- **Container Orchestration：Docker / Docker Compose**

### 6.2 選型原因

- 容器化整合容易
- 適合建立多個獨立出口
- 容易與 relay 共用網路命名空間
- 支援健康檢查與狀態查詢
- 易於由後端透過 Docker API 管理生命週期

### 6.3 每個出口的部署形式

每一個出口包含：

- 一個 `gluetun-{name}`
- 一個 `relay-{name}`

### 6.4 WebUI 對出口的管理範圍

WebUI 需要能管理：

- 容器建立
- 容器刪除
- 容器啟動
- 容器停止
- 容器重啟
- 狀態查詢
- 錯誤查看

---

## 7. API 設計

## 7.1 Rules API

### 新增規則

```http
POST /api/rules
```

```json
{
  "pattern": "+.openai.com",
  "type": "suffix",
  "exit": "us",
  "enabled": true,
  "priority": 100
}
```

### 刪除規則

```http
DELETE /api/rules/{id}
```

### 查詢規則

```http
GET /api/rules
```

### 更新規則

```http
PUT /api/rules/{id}
```

---

## 7.2 Subscriptions API

### 新增訂閱

```http
POST /api/subscriptions
```

```json
{
  "name": "AI Sites",
  "url": "https://example.com/ai.list",
  "exit": "us",
  "enabled": true,
  "interval_minutes": 360
}
```

### 更新訂閱

```http
PUT /api/subscriptions/{id}
```

### 刪除訂閱

```http
DELETE /api/subscriptions/{id}
```

### 手動同步

```http
POST /api/subscriptions/{id}/sync
```

---

## 7.3 Exits API

### 新增出口

```http
POST /api/exits
```

```json
{
  "name": "jp",
  "runtime": "gluetun",
  "protocol": "wireguard",
  "provider": "protonvpn",
  "country": "Japan",
  "city": "Tokyo",
  "wireguard_private_key": "***",
  "enabled": true
}
```

### 查詢出口列表

```http
GET /api/exits
```

### 啟動出口

```http
POST /api/exits/{id}/start
```

### 停止出口

```http
POST /api/exits/{id}/stop
```

### 重啟出口

```http
POST /api/exits/{id}/restart
```

### 刪除出口

```http
DELETE /api/exits/{id}
```

### 測試出口

```http
POST /api/exits/{id}/test
```

### 查詢出口 logs

```http
GET /api/exits/{id}/logs
```

### 查詢出口狀態

```http
GET /api/exits/{id}/status
```

---

## 7.4 Publish API

### 預覽配置

```http
POST /api/publish/preview
```

### 驗證配置

```http
POST /api/publish/validate
```

### 套用配置

```http
POST /api/publish/apply
```

### 回滾配置

```http
POST /api/publish/rollback
```

---

## 8. 資料庫設計

建議使用 PostgreSQL。

## 8.1 exits

| 欄位 | 型別 | 說明 |
|---|---|---|
| id | bigint | 主鍵 |
| name | varchar | 出口名稱 |
| runtime | varchar | gluetun |
| protocol | varchar | wireguard/openvpn |
| provider | varchar | VPN 供應商 |
| country | varchar | 國家 |
| city | varchar | 城市 |
| status | varchar | running/stopped/unhealthy |
| public_ip | varchar | 目前出口 IP |
| enabled | bool | 是否啟用 |
| config_json | jsonb | 出口設定 |
| created_at | timestamptz | 建立時間 |
| updated_at | timestamptz | 更新時間 |

## 8.2 rules

| 欄位 | 型別 | 說明 |
|---|---|---|
| id | bigint | 主鍵 |
| pattern | text | 網域規則 |
| type | varchar | exact/suffix |
| exit_id | bigint | 對應出口 |
| priority | int | 優先序 |
| source | varchar | manual/subscription |
| enabled | bool | 是否啟用 |
| created_at | timestamptz | 建立時間 |
| updated_at | timestamptz | 更新時間 |

## 8.3 subscriptions

| 欄位 | 型別 | 說明 |
|---|---|---|
| id | bigint | 主鍵 |
| name | varchar | 訂閱名稱 |
| url | text | 訂閱位址 |
| exit_id | bigint | 綁定出口 |
| enabled | bool | 是否啟用 |
| interval_minutes | int | 更新週期 |
| last_sync_at | timestamptz | 上次更新時間 |
| last_sync_status | varchar | 上次狀態 |
| last_error | text | 上次錯誤 |
| created_at | timestamptz | 建立時間 |
| updated_at | timestamptz | 更新時間 |

## 8.4 publish_history

| 欄位 | 型別 | 說明 |
|---|---|---|
| id | bigint | 主鍵 |
| version | varchar | 配置版本 |
| status | varchar | success/failed/rollback |
| message | text | 說明 |
| created_at | timestamptz | 發布時間 |

## 8.5 exit_runtime_events

| 欄位 | 型別 | 說明 |
|---|---|---|
| id | bigint | 主鍵 |
| exit_id | bigint | 出口 ID |
| action | varchar | create/start/stop/restart/delete |
| status | varchar | success/failed |
| message | text | 執行結果 |
| created_at | timestamptz | 時間 |

---

## 9. 容器控制設計

## 9.1 後端控制職責

後端需負責以下容器控制：

- 建立 Gluetun 容器
- 建立 relay 容器
- 啟動容器
- 停止容器
- 重啟容器
- 刪除容器
- 檢查容器狀態
- 讀取容器 logs

## 9.2 控制方式

建議使用：

- Docker Engine API
- Docker SDK

避免直接大量 shell out 到 `docker` CLI 作為唯一手段。

## 9.3 容器建立流程

1. 建立出口資料
2. 產生 Gluetun container config
3. 產生 relay container config
4. 呼叫 Docker API 建立容器
5. 啟動 Gluetun
6. 啟動 relay
7. 執行健康檢查
8. 更新資料庫狀態

## 9.4 容器刪除流程

1. 檢查是否仍被規則引用
2. 停止 relay
3. 停止 Gluetun
4. 刪除 relay
5. 刪除 Gluetun
6. 清理對應配置
7. 更新資料庫狀態

---

## 10. 設定發布流程

1. 收集所有有效規則
2. 依優先序排序
3. 產生內部映射表
4. 產生 Nginx 設定檔
5. 執行語法驗證
6. 驗證成功後 reload Nginx
7. 記錄發布結果
8. 失敗時保留上一版本可回滾

---

## 11. 安全設計

### 11.1 WebUI 安全

- 管理者登入驗證
- Session 或 JWT 驗證
- 權限分級
- 重要操作二次確認

### 11.2 憑證安全

- WireGuard Private Key 不以明文回傳前端
- 敏感欄位資料庫需加密儲存
- logs 中避免輸出完整金鑰

### 11.3 出口安全

- 不允許未知出口直接被規則引用
- 刪除出口前檢查相依規則
- 預設拒絕未定義域名或導向 default 出口

### 11.4 配置安全

- 發布前必須執行語法檢查
- 所有發布需保留審計紀錄
- 配置需可回滾

---

## 12. 技術棧建議

## 12.1 Backend

建議：

- Go + Fiber / Gin

原因：

- 容易處理高併發 API
- 容易整合 Docker SDK
- 適合做控制平面

## 12.2 Frontend

建議：

- React
- Tailwind CSS
- shadcn/ui

## 12.3 Data Plane

- Nginx
- Nginx stream module

## 12.4 Runtime

- Docker
- Docker Compose

## 12.5 Database

- PostgreSQL

---

## 13. MVP 範圍

### Phase 1

- 支援多個 Gluetun 出口
- 支援手動規則
- 支援 Nginx SNI 分流
- 支援 WebUI 新增/刪除/啟停出口

### Phase 2

- 支援遠端 `.list` 訂閱
- 支援自動更新
- 支援規則發布與回滾
- 支援出口健康檢查

### Phase 3

- 支援完整日誌查詢
- 支援使用統計
- 支援 RBAC
- 支援多使用者管理

---

## 14. 成功標準

- 新增出口後可在 1 分鐘內完成容器建立與啟動
- 規則更新後可在 3 秒內完成配置套用
- 訂閱同步成功率高於 99%
- 單次 Nginx reload 不中斷既有連線
- WebUI 可完整管理 Gluetun 容器生命週期

---

## 15. 最終產品描述

本系統是一套 **可視化、多出口、規則驅動、支援訂閱更新的 SNI 分流平台**。

使用者可透過 WebUI 管理規則、訂閱與 VPN 出口，系統負責將規則編譯為 Nginx 路由配置，並將不同網域自動導向指定的 Gluetun VPN 出口。

本系統同時具備 **出口容器生命週期管理能力**，可直接在 WebUI 中建立、刪除、啟動、停止與觀察 Gluetun 容器，形成完整的控制平面與資料平面架構。

