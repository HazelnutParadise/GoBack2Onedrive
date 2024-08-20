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

		if info.IsDir() {
			return nil
		}

		relPath := path[len(source):]
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

func uploadFileToOneDrive(accessToken, filePath, oneDriveFolder string) error {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("無法讀取檔案: %v", err)
	}

	fileName := oneDriveFolder + "/" + getFileName(filePath)
	uploadURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/content", fileName)

	req, err := http.NewRequest("PUT", uploadURL, bytes.NewReader(fileData))
	if err != nil {
		return fmt.Errorf("無法建立 HTTP 請求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{}

	// 重試機制
	for {
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("上傳請求失敗: %v\n", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusCreated {
				fmt.Println("成功上傳檔案到 OneDrive.")
				return nil
			} else {
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("上傳失敗: %v - %s\n", resp.Status, string(body))
			}
		}

		// 如果失敗，等待 10 秒後重試
		fmt.Println("10 秒後重試...")
		time.Sleep(10 * time.Second)
	}
}

func getFileName(filePath string) string {
	return filePath[strings.LastIndex(filePath, "/")+1:]
}

func listBackupsOnOneDrive(accessToken, oneDriveFolder string) ([]OneDriveItem, error) {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/root:/%s:/children", oneDriveFolder)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("無法建立 HTTP 請求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API請求失敗: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API請求錯誤: %v - %s", resp.Status, string(body))
	}

	var oneDriveResp OneDriveResponse
	err = json.NewDecoder(resp.Body).Decode(&oneDriveResp)
	if err != nil {
		return nil, fmt.Errorf("無法解析API響應: %v", err)
	}

	return oneDriveResp.Value, nil
}

func deleteBackupOnOneDrive(accessToken, itemId string) error {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/drive/items/%s", itemId)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("無法建立 HTTP 請求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
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

func cleanOldBackupsOnOneDrive(accessToken, oneDriveFolder string, maxBackups int) error {
	backups, err := listBackupsOnOneDrive(accessToken, oneDriveFolder)
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
		err := deleteBackupOnOneDrive(accessToken, backups[i].Id)
		if err != nil {
			fmt.Printf("刪除備份錯誤: %v\n", err)
		} else {
			fmt.Printf("已刪除舊備份: %s\n", backups[i].Name)
		}
	}

	return nil
}

func main() {
	sourceDir := "/app/data" // 容器內的 data 資料夾
	timestamp := time.Now().Format("20060102-150405")
	targetZip := fmt.Sprintf("/app/backups/backup-%s.zip", timestamp)
	accessToken := os.Getenv("ACCESS_TOKEN")
	oneDriveFolder := os.Getenv("ONEDRIVE_DESTINATION_FOLDER") // 讀取目的地資料夾
	if oneDriveFolder == "" {
		oneDriveFolder = "backups" // 如果沒有設定，預設為 "backups"
	}
	maxBackups := 5 // 保留最多5個備份

	// 壓縮資料夾
	err := zipFolder(sourceDir, targetZip)
	if err != nil {
		fmt.Printf("壓縮資料夾錯誤: %v\n", err)
		return
	}

	fmt.Println("資料夾已成功壓縮。")

	// 清理OneDrive上的舊備份
	err = cleanOldBackupsOnOneDrive(accessToken, oneDriveFolder, maxBackups)
	if err != nil {
		fmt.Printf("清理舊備份錯誤: %v\n", err)
		return
	}

	// 上傳壓縮檔案
	err = uploadFileToOneDrive(accessToken, targetZip, oneDriveFolder)
	if err != nil {
		fmt.Printf("上傳錯誤: %v\n", err)
		return
	}

	// 上傳成功後刪除本地備份檔案
	err = os.Remove(targetZip)
	if err != nil {
		fmt.Printf("刪除本地備份檔案錯誤: %v\n", err)
		return
	}

	fmt.Println("已成功刪除本地備份檔案。")
}
