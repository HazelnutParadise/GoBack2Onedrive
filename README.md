# GoBack2Onedrive
Auto backup files to Onedrive.

GoBack2Onedrive 是一個使用 Go 語言編寫的工具，能夠將指定的資料夾壓縮成ZIP文件後上傳到 OneDrive，並且可以根據配置自動清理舊的備份文件。該工具支持分塊上傳大文件，並且會在上傳過程中自動重試直到成功。此工具可以配置為在 Docker 容器中運行，並通過 Docker Compose 定期執行備份操作。

## 功能特點

- 自動壓縮資料夾並上傳到 OneDrive。
- 支援分塊上傳大文件。
- 根據配置自動清理舊的備份文件。
- 上傳失敗自動重試直到成功。
- 上傳成功後自動清空本地的備份文件。

## 安裝

1. **克隆倉庫：**

   ```bash
   git clone https://github.com/yourusername/goback2onedrive.git
   cd goback2onedrive
   ```

2. **構建 Docker 映像：**

   在項目根目錄下運行以下命令來構建 Docker 映像：

   ```bash
   docker build -t goback2onedrive:latest .
   ```

3. **配置 Docker Compose：**

   建立 `docker-compose.yml` 文件（內容可參考docker-compose-demo.yml），設置需要備份的資料夾路徑以及 OneDrive API 的相關環境變數。

## 使用

1. **環境變數設置：**

   在 `docker-compose.yml` 文件中，設置以下環境變數：

   - `CLIENT_ID`: 你的 OneDrive 應用程式的客戶端 ID。
   - `CLIENT_SECRET`: 你的 OneDrive 應用程式的客戶端密鑰。
   - `TENANT_ID`: 你的 OneDrive 租戶 ID。
   - `DRIVE_ID`: 你的 OneDrive 驅動器 ID。
   - `ONEDRIVE_DESTINATION_FOLDER`: 備份文件上傳到 OneDrive 的目標資料夾。
   - `MAX_BACKUPS`: 要在 OneDrive 保留的最大備份數量。

2. **運行 Docker Compose：**

   使用以下命令來運行容器並啟動備份任務：

   ```bash
   docker-compose up -d
   ```

   此命令將根據 `docker-compose.yml` 中的配置定期進行備份並上傳到 OneDrive。

3. **檢查日誌：**

   如果需要查看容器的日誌，可以運行以下命令：

   ```bash
   docker logs -f goback2onedrive-backup
   ```

4. **清理本地備份文件：**

   程式會自動在成功上傳後清理 `/app/backups` 資料夾中的本地備份文件。

## 文件結構

- `GoBack2Onedrive.go`: 主程式文件，包含壓縮、上傳和清理備份的邏輯。
- `Dockerfile`: 定義了如何構建 GoBack2Onedrive 的 Docker 映像。
- `docker-compose.yml`: 用於配置和管理 Docker 容器的 Compose 文件。

## 貢獻

如果你有任何想法或問題，歡迎提交 issue 或 pull request。

## 許可證

這個專案使用 [MIT 許可證](LICENSE)。
