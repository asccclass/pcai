package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/ollama/ollama/api"
)

// --- 1. 定義介面 ---

type AgentTool interface {
	Name() string
	Definition() api.Tool
	Run(argsJSON string) (string, error)
}

// 實作 PythonSandboxTool
type PythonSandboxTool struct {
	dockerClient  *client.Client
	workspacePath string
}

// 輔助函式：寫入除錯日誌
func debugLog(format string, args ...interface{}) {
	f, err := os.OpenFile("sandbox_debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, msg))
}

// NewPythonSandboxTool 建立並初始化工具
func NewPythonSandboxTool(workspacePath string) (*PythonSandboxTool, error) {
	debugLog("Initializing PythonSandboxTool with workspace: %s", workspacePath)
	// 驗證 workspacePath 是否存在
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		debugLog("Error: Workspace path does not exist")
		return nil, fmt.Errorf("workspace path does not exist: %s", workspacePath)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		debugLog("Error creating Docker client: %v", err)
		return nil, err
	}

	// 測試 Docker 連線
	if _, err := cli.Ping(context.Background()); err != nil {
		debugLog("Error pinging Docker: %v", err)
		return nil, fmt.Errorf("Docker connection failed: %v", err)
	}
	debugLog("Docker client initialized successfully")

	return &PythonSandboxTool{
		dockerClient:  cli,
		workspacePath: workspacePath,
	}, nil
}

func (t *PythonSandboxTool) Name() string {
	return "run_python_code"
}

func (t *PythonSandboxTool) Definition() api.Tool {
	return api.Tool{
		Type: "function",
		Function: api.ToolFunction{
			Name:        t.Name(),
			Description: "在安全的 Docker 沙盒環境中執行 Python 程式碼。已掛載 Workspace 目錄至 /mnt/workspace，可讀寫該目錄下的檔案。若需要執行長時間任務，請將 background 設為 true。",
			Parameters: func() api.ToolFunctionParameters {
				var props api.ToolPropertiesMap
				js := `{
					"code": {
						"type": "string",
						"description": "要執行的 Python 原始碼內容"
					},
					"background": {
						"type": "boolean",
						"description": "是否在背景執行 (預設 false)。若為 true，會立即回傳 Container ID，不會等待執行結果。"
					},
					"required": []string{"code"},
				}`
				_ = json.Unmarshal([]byte(js), &props)

				return api.ToolFunctionParameters{
					Type:       "object",
					Properties: &props,
					Required:   []string{"code"},
				}
			}(),
		},
	}
}

func (t *PythonSandboxTool) Run(argsJSON string) (string, error) {
	debugLog("Run called with args: %s", argsJSON)
	// 1. 解析參數
	var args struct {
		Code       string `json:"code"`
		Background bool   `json:"background"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		debugLog("Error parsing args: %v", err)
		return "", fmt.Errorf("參數解析失敗: %v", err)
	}

	ctx := context.Background()

	// 2. 建立臨時檔案 (在 Workspace 內的 .sandbox_temp 目錄)
	// 確保 .sandbox_temp 目錄存在
	tempDir := filepath.Join(t.workspacePath, ".sandbox_temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		debugLog("Error creating temp dir: %v", err)
		return "", fmt.Errorf("無法建立臨時目錄: %v", err)
	}

	// 建立唯一的 script 檔案
	timestamp := time.Now().UnixNano()
	scriptName := fmt.Sprintf("script_%d.py", timestamp)
	scriptPath := filepath.Join(tempDir, scriptName)

	if err := os.WriteFile(scriptPath, []byte(args.Code), 0644); err != nil {
		debugLog("Error writing script file: %v", err)
		return "", fmt.Errorf("無法寫入腳本檔案: %v", err)
	}

	// 定義清理函數
	cleanup := func() {
		os.Remove(scriptPath)
	}

	// 3. 配置容器
	// 轉換為絕對路徑以供 Docker Bind Mount 使用
	absScriptPath, err := filepath.Abs(scriptPath)
	if err != nil {
		cleanup()
		debugLog("Error getting absolute path for script: %v", err)
		return "", fmt.Errorf("無法取得腳本絕對路徑: %v", err)
	}

	absWorkspacePath, err := filepath.Abs(t.workspacePath)
	if err != nil {
		cleanup()
		debugLog("Error getting absolute path for workspace: %v", err)
		return "", fmt.Errorf("無法取得 Workspace 絕對路徑: %v", err)
	}

	containerConfig := &container.Config{
		Image:           "python:3.9-slim",
		Cmd:             []string{"python", "/mnt/script.py"}, // 執行掛載的腳本
		NetworkDisabled: true,                                 // 禁止聯網
		WorkingDir:      "/mnt/workspace",                     // 設定工作目錄
	}

	hostConfig := &container.HostConfig{
		Binds: []string{
			// 掛載腳本 (唯讀)
			fmt.Sprintf("%s:/mnt/script.py:ro", absScriptPath),
			// 掛載 Workspace (可讀寫)
			fmt.Sprintf("%s:/mnt/workspace", absWorkspacePath),
		},
		AutoRemove: false,                                          // 必須設為 false，否則執行完瞬間就被刪除，讀不到 logs
		Resources:  container.Resources{Memory: 128 * 1024 * 1024}, // 限制 128MB
	}

	// 3.5. 檢查並拉取映像檔
	imageName := "python:3.9-slim"
	_, _, err = t.dockerClient.ImageInspectWithRaw(ctx, imageName)
	if client.IsErrNotFound(err) {
		debugLog("Image %s not found, pulling...", imageName)
		reader, err := t.dockerClient.ImagePull(ctx, imageName, types.ImagePullOptions{})
		if err != nil {
			cleanup()
			debugLog("Error pulling image: %v", err)
			return "", fmt.Errorf("無法拉取映像檔 %s: %v", imageName, err)
		}
		defer reader.Close()
		// 读取 Pull 输出以确保拉取完成
		io.Copy(io.Discard, reader)
		debugLog("Image pulled successfully")
	} else if err != nil {
		cleanup()
		debugLog("Error inspecting image: %v", err)
		return "", fmt.Errorf("檢查映像檔失敗: %v", err)
	}

	// 4. 執行容器
	debugLog("Creating container...")
	resp, err := t.dockerClient.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		cleanup()
		debugLog("Error creating container: %v", err)
		return "", fmt.Errorf("無法建立容器: %v", err)
	}

	// 確保容器最後會被移除 (因為 AutoRemove=false)
	removeContainer := func() {
		// 給予一點緩衝時間讓 logs讀取完畢 (雖然正常流程是 defer LIFO)
		_ = t.dockerClient.ContainerRemove(context.Background(), resp.ID, types.ContainerRemoveOptions{Force: true})
	}

	debugLog("Starting container %s...", resp.ID[:12])
	if err := t.dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		cleanup()
		removeContainer() // 啟動失敗也要移除
		debugLog("Error starting container: %v", err)
		return "", fmt.Errorf("無法啟動容器: %v", err)
	}

	// 5. 判斷是否背景執行
	if args.Background {
		debugLog("Background task started. Container ID: %s", resp.ID)
		go func(containerID string) {
			defer cleanup()
			defer func() {
				_ = t.dockerClient.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{Force: true})
			}()

			statusCh, errCh := t.dockerClient.ContainerWait(context.Background(), containerID, container.WaitConditionNotRunning)
			select {
			case <-errCh:
			case <-statusCh:
			case <-time.After(1 * time.Hour):
				_ = t.dockerClient.ContainerKill(context.Background(), containerID, "SIGKILL")
			}
		}(resp.ID)
		return fmt.Sprintf("背景任務已啟動。Container ID: %s", resp.ID), nil
	}

	// 同步執行：等待並清理
	defer cleanup()
	defer removeContainer() // 確保同步執行也會移除容器

	// 5. 等待並獲取輸出 (含超時控制)
	debugLog("Waiting for container...")
	statusCh, errCh := t.dockerClient.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var statusCode int64
	select {
	case err := <-errCh:
		debugLog("Error waiting for container: %v", err)
		return "", fmt.Errorf("容器執行錯誤: %v", err)
	case status := <-statusCh:
		statusCode = status.StatusCode
		debugLog("Container finished with status code: %d", statusCode)
	case <-time.After(15 * time.Second):
		_ = t.dockerClient.ContainerKill(ctx, resp.ID, "SIGKILL")
		debugLog("Container execution timed out")
		return "", fmt.Errorf("腳本執行超時 (15s)")
	}

	out, err := t.dockerClient.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		debugLog("Error getting logs: %v", err)
		return "", fmt.Errorf("無法讀取日誌: %v", err)
	}
	defer out.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, out); err != nil {
		debugLog("Error copying logs: %v", err)
		return "", fmt.Errorf("讀取日誌失敗: %v", err)
	}

	result := stdout.String()
	errData := stderr.String()
	debugLog("Stdout: %s", result)
	debugLog("Stderr: %s", errData)

	if statusCode != 0 {
		return "", fmt.Errorf("腳本執行失敗 (Exit Code: %d)\nStderr: %s\nStdout: %s", statusCode, errData, result)
	}

	if errData != "" {
		// 雖然 exit code 0，但有 stderr (警告等)，附加上去
		return fmt.Sprintf("%s\n(Stderr: %s)", result, errData), nil
	}
	return result, nil
}
