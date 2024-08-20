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

// 获取访问令牌
func (client *OneDriveClient) GetAccessToken() error {
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
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("获取访问令牌失败: %s", resp.Status)
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return err
	}

	client.AccessToken = result["access_token"].(string)
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
		return nil, fmt.Errorf("无法建立 HTTP 请求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.AccessToken)

	clientHTTP := &http.Client{}
	resp, err := clientHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 请求错误: %v - %s", resp.Status, string(body))
	}

	var oneDriveResp OneDriveResponse
	err = json.NewDecoder(resp.Body).Decode(&oneDriveResp)
	if err != nil {
		return nil, fmt.Errorf("无法解析 API 响应: %v", err)
	}

	return oneDriveResp.Value, nil
}

// 删除 OneDrive 中的文件
func (client *OneDriveClient) DeleteBackupOnOneDrive(itemId string) error {
	err := client.GetAccessToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/items/%s", client.DriveID, itemId)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("无法建立 HTTP 请求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.AccessToken)

	clientHTTP := &http.Client{}
	resp, err := clientHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("删除请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除失败: %v - %s", resp.Status, string(body))
	}

	return nil
}

// 上传文件到 OneDrive
func (client *OneDriveClient) UploadFileToOneDrive(filePath, oneDriveFolder string) error {
	err := client.GetAccessToken()
	if err != nil {
		return err
	}

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("无法读取文件: %v", err)
	}

	fileName := oneDriveFolder + "/" + filepath.Base(filePath)
	uploadURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/drives/%s/root:/%s:/content", client.DriveID, fileName)

	req, err := http.NewRequest("PUT", uploadURL, bytes.NewReader(fileData))
	if err != nil {
		return fmt.Errorf("无法建立 HTTP 请求: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.AccessToken)
	req.Header.Set("Content-Type", "application/octet-stream")

	clientHTTP := &http.Client{}

	// 重试机制
	for {
		resp, err := clientHTTP.Do(req)
		if err != nil {
			fmt.Printf("上传请求失败: %v\n", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
				fmt.Println("成功上传文件到 OneDrive.")
				return nil
			} else {
				body, _ := io.ReadAll(resp.Body)
				fmt.Printf("上传失败: %v - %s\n", resp.Status, string(body))
			}
		}

		// 如果失败，等待 10 秒后重试
		fmt.Println("10 秒后重试...")
		time.Sleep(10 * time.Second)
	}
}

// 清理旧的备份文件
func (client *OneDriveClient) CleanOldBackups(oneDriveFolder string, maxBackups int) error {
	backups, err := client.ListBackupsOnOneDrive(oneDriveFolder)
	if err != nil {
		return err
	}

	if len(backups) <= maxBackups {
		return nil
	}

	// 按最后修改时间排序
	sort.Slice(backups, func(i, j int) bool {
		timeI, _ := time.Parse(time.RFC3339, backups[i].LastModified)
		timeJ, _ := time.Parse(time.RFC3339, backups[j].LastModified)
		return timeI.Before(timeJ)
	})

	for i := 0; i < len(backups)-maxBackups; i++ {
		err := client.DeleteBackupOnOneDrive(backups[i].Id)
		if err != nil {
			fmt.Printf("删除备份错误: %v\n", err)
		} else {
			fmt.Printf("已删除旧备份: %s\n", backups[i].Name)
		}
	}

	return nil
}

// 压缩文件夹
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

// 主函数
func main() {
	fmt.Println("GoBack2Onedrive program started...")
	client := OneDriveClient{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		TenantID:     os.Getenv("TENANT_ID"),
		DriveID:      os.Getenv("DRIVE_ID"),
	}

	sourceDir := "/app/data" // 容器内的 data 文件夹
	timestamp := time.Now().Format("20060102-150405")
	targetZip := fmt.Sprintf("/app/backups/backup-%s.zip", timestamp)
	oneDriveFolder := os.Getenv("ONEDRIVE_DESTINATION_FOLDER")
	if oneDriveFolder == "" {
		oneDriveFolder = "backups" // 如果没有设置，默认保存到 "backups" 文件夹
	}

	// 从环境变量读取最大备份数
	maxBackupsStr := os.Getenv("MAX_BACKUPS")
	maxBackups, err := strconv.Atoi(maxBackupsStr)
	if err != nil || maxBackups <= 0 {
		maxBackups = 5 // 默认保留最多 5 个备份
	}

	// 压缩文件夹
	err = zipFolder(sourceDir, targetZip)
	if err != nil {
		fmt.Printf("压缩文件夹错误: %v\n", err)
		return
	}

	fmt.Println("文件夹已成功压缩。")

	// 清理 OneDrive 上的旧备份
	err = client.CleanOldBackups(oneDriveFolder, maxBackups)
	if err != nil {
		fmt.Printf("清理旧备份错误: %v\n", err)
		return
	}

	// 上传压缩文件
	err = client.UploadFileToOneDrive(targetZip, oneDriveFolder)
	if err != nil {
		fmt.Printf("上传错误: %v\n", err)
		return
	}

	// 上传成功后删除本地备份文件
	err = os.Remove(targetZip)
	if err != nil {
		fmt.Printf("删除本地备份文件错误: %v\n", err)
		return
	}

	fmt.Println("已成功删除本地备份文件。")
}
