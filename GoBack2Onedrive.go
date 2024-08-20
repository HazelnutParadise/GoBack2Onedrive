package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type OneDriveItem struct {
	Name         string `json:"name"`
	LastModified string `json:"lastModifiedDateTime"`
	Id           string `json:"id"`
}

type OneDriveResponse struct {
	Value []OneDriveItem `json:"value"`
}

type OneDriveClient struct {
	ClientID     string
	ClientSecret string
	TenantID     string
	DriveID      string
	AccessToken  string
}

type UploadSession struct {
	UploadURL string `json:"uploadUrl"`
}

const chunkSize = 10 * 1024 * 1024 // 10 MB

// 取得存取令牌，新增重試機制
func (client *OneDriveClient) GetAccessToken() error {
	for {
		url := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", client.TenantID)
		payload := strings.NewReader(fmt.Sprintf(
			"grant_type=client_credentials&client_id=%s&client_secret=%s&scope=https://graph.microsoft.com/.default",
			client.ClientID, client.ClientSecret))

		req, err := http.NewRequest("POST", url, payload)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("取得存取權杖請求失敗: %v\n", err)
		} else {
			defer resp.Body.Close()

			if resp.StatusCode == 200 {
				var result map[string]interface{}
				err = json.NewDecoder(resp.Body).Decode(&result)
				if err != nil {
					fmt.Printf("解析存取令牌回應錯誤: %v\n", err)
				} else {
					client.AccessToken = result["access_token"].(string)
					return nil
				}
			} else {
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("取得存取權杖失敗: %s - %s\n", resp.Status, string(body))
			}
		}

		// 如果失敗，等待 30 秒後重試
		fmt.Println("30 秒後重試取得存取權杖...")
		time.Sleep(30 * time.Second)
	}
}

// 建立 OneDrive 資料夾
func (client *OneDriveClient) CreateOneDriveFolder(folderPath string) error {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/root/children", client.DriveID)

	folderName := filepath.Base(folderPath)
	payload := map[string]interface{}{
		"name":                              folderName,
		"folder":                            map[string]interface{}{},
		"@microsoft.graph.conflictBehavior": "rename",
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("無法編碼 JSON: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("無法建立 HTTP 請求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	clientHTTP := &http.Client{}
	resp, err := clientHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("API 請求失敗: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("建立資料夾失敗: %v - %s", resp.Status, string(body))
	}

	fmt.Printf("資料夾 %s 建立成功。\n", folderPath)
	return nil
}

// 列出 OneDrive 文件夹中的文件
func (client *OneDriveClient) ListBackupsOnOneDrive(oneDriveFolder string) ([]OneDriveItem, error) {
	err := client.GetAccessToken()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/root:/%s:/children", client.DriveID, oneDriveFolder)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("無法建立 HTTP 請求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.AccessToken)

	clientHTTP := &http.Client{}
	resp, err := clientHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 請求失敗: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			fmt.Println("目標資料夾未找到，正在建立...")
			err = client.CreateOneDriveFolder(oneDriveFolder)
			if err != nil {
				return nil, err
			}
			fmt.Println("資料夾已創建，自動重新執行...")
			return client.ListBackupsOnOneDrive(oneDriveFolder) // 重新运行 List 操作
		}
		return nil, fmt.Errorf("API 請求錯誤: %v - %s", resp.Status, string(body))
	}

	var oneDriveResp OneDriveResponse
	err = json.NewDecoder(resp.Body).Decode(&oneDriveResp)
	if err != nil {
		return nil, fmt.Errorf("無法解析 API 回應: %v", err)
	}

	return oneDriveResp.Value, nil
}

// 刪除 OneDrive 中的文件
func (client *OneDriveClient) DeleteBackupOnOneDrive(itemId string) error {
	err := client.GetAccessToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/items/%s", client.DriveID, itemId)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("無法建立 HTTP 請求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.AccessToken)

	clientHTTP := &http.Client{}
	resp, err := clientHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("刪除請求失敗: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("刪除失敗: %v - %s", resp.Status, string(body))
	}

	return nil
}

// 创建上传会话
func (client *OneDriveClient) CreateUploadSession(fileName, oneDriveFolder string) (*UploadSession, error) {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/root:/%s/%s:/createUploadSession", client.DriveID, oneDriveFolder, fileName)

	payload := map[string]interface{}{
		"item": map[string]interface{}{
			"@microsoft.graph.conflictBehavior": "rename",
			"name":                              fileName,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("無法編碼 JSON: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("無法建立 HTTP 請求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	clientHTTP := &http.Client{}
	resp, err := clientHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 請求失敗: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("建立上傳會話失敗: %v - %s", resp.Status, string(body))
	}

	var uploadSession UploadSession
	err = json.NewDecoder(resp.Body).Decode(&uploadSession)
	if err != nil {
		return nil, fmt.Errorf("解析上傳會話回應錯誤: %v", err)
	}

	return &uploadSession, nil
}

// 分块上传文件
func (client *OneDriveClient) UploadFileInChunks(filePath, oneDriveFolder string) error {
	err := client.GetAccessToken()
	if err != nil {
		return err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("無法開啟文件: %v", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("無法取得文件資訊: %v", err)
	}

	fileName := filepath.Base(filePath)
	uploadSession, err := client.CreateUploadSession(fileName, oneDriveFolder)
	if err != nil {
		return err
	}

	buffer := make([]byte, chunkSize)
	var start, end int64
	totalSize := fileInfo.Size()

	for start = 0; start < totalSize; start += chunkSize {
		end = start + chunkSize
		if end > totalSize {
			end = totalSize
		}

		_, err := file.Seek(start, io.SeekStart)
		if err != nil {
			return fmt.Errorf("無法定位文件指針: %v", err)
		}

		bytesRead, err := file.Read(buffer[:end-start])
		if err != nil {
			return fmt.Errorf("讀取文件資料失敗: %v", err)
		}

		uploadURL := uploadSession.UploadURL
		req, err := http.NewRequest("PUT", uploadURL, bytes.NewReader(buffer[:bytesRead]))
		if err != nil {
			return fmt.Errorf("無法建立 HTTP 請求: %v", err)
		}

		req.Header.Set("Authorization", "Bearer "+client.AccessToken)
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, totalSize))

		clientHTTP := &http.Client{}
		resp, err := clientHTTP.Do(req)
		if err != nil {
			return fmt.Errorf("上傳區塊請求失敗: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("上傳區塊失敗: %v - %s", resp.Status, string(body))
		}

		fmt.Printf("[上傳進度：%d ]已成功上傳 %d-%d 位元組，共 %d 位元組\n", (end-1)/totalSize*100, start, end-1, totalSize)
	}

	fmt.Println("檔案上傳完成。")
	return nil
}

// 清理舊的備份檔
func (client *OneDriveClient) CleanOldBackups(oneDriveFolder string, maxBackups int) error {
	backups, err := client.ListBackupsOnOneDrive(oneDriveFolder)
	if err != nil {
		return err
	}

	if len(backups) <= maxBackups {
		return nil
	}

	// 按最後修改時間排序
	sort.Slice(backups, func(i, j int) bool {
		timeI, _ := time.Parse(time.RFC3339, backups[i].LastModified)
		timeJ, _ := time.Parse(time.RFC3339, backups[j].LastModified)
		return timeI.Before(timeJ)
	})

	for i := 0; i < len(backups)-maxBackups; i++ {
		err := client.DeleteBackupOnOneDrive(backups[i].Id)
		if err != nil {
			fmt.Printf("刪除備份錯誤: %v\n", err)
		} else {
			fmt.Printf("已刪除舊備份: %s\n", backups[i].Name)
		}
	}

	return nil
}

// 壓縮資料夾
func zipFolder(source, target string) error {
	zipFile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 忽略符號連結
		if info.Mode()&os.ModeSymlink != 0 {
			fmt.Printf("Skipping symlink: %s\n", path)
			return nil
		}

		// 檢查文件或資料夾是否存在
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("Skipping non-existent path: %s\n", path)
			return nil
		}

		// 取得相對路徑，以便後續比較目錄結構
		relPath := path[len(source):]

		// 排除不需要備份的目錄
		if strings.Contains(relPath, ".Trash") ||
			strings.Contains(relPath, "lost+found") ||
			strings.Contains(relPath, "backups") {
			fmt.Printf("Skipping: %s\n", path)
			return nil
		}

		if info.IsDir() {
			return nil
		}

		zipFile, err := writer.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(zipFile, file)
		return err
	})

	return err
}

// 主函數
func main() {
	fmt.Println("GoBack2Onedrive program started...")
	client := OneDriveClient{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		TenantID:     os.Getenv("TENANT_ID"),
		DriveID:      os.Getenv("DRIVE_ID"),
	}

	sourceDir := "/app/data" // 容器內的 data 資料夾
	timestamp := time.Now().Format("20060102-150405")
	targetZip := fmt.Sprintf("/app/backups/backup-%s.zip", timestamp)
	oneDriveFolder := os.Getenv("ONEDRIVE_DESTINATION_FOLDER")
	if oneDriveFolder == "" {
		oneDriveFolder = "backups" // 如果沒有設置，預設儲存到 "backups" 資料夾
	}

	// 從環境變數讀取最大備份數
	maxBackupsStr := os.Getenv("MAX_BACKUPS")
	maxBackups, err := strconv.Atoi(maxBackupsStr)
	if err != nil || maxBackups <= 0 {
		maxBackups = 5 // 預設保留最多 5 個備份
	}

	// 壓縮資料夾
	err = zipFolder(sourceDir, targetZip)
	if err != nil {
		fmt.Printf("壓縮資料夾錯誤: %v\n", err)
		return
	}

	fmt.Println("資料夾已成功壓縮。")

	// 清理 OneDrive 上的舊備份
	err = client.CleanOldBackups(oneDriveFolder, maxBackups)
	if err != nil {
		fmt.Printf("清理舊備份錯誤: %v\n", err)
		return
	}

	// 分塊上傳壓縮文件
	err = client.UploadFileInChunks(targetZip, oneDriveFolder)
	if err != nil {
		fmt.Printf("上傳錯誤: %v\n", err)
		return
	}

	// 上傳成功後刪除本機備份文件
	err = os.Remove(targetZip)
	if err != nil {
		fmt.Printf("刪除本機備份檔案錯誤: %v\n", err)
		return
	}

	fmt.Println("已成功刪除本機備份檔案。")
}
