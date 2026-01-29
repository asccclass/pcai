package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ollama/ollama/api"
)

// VideoConverterTool 實作 Ollama 工具介面
type VideoConverterTool struct{}

// Arguments 定義 LLM 傳入的 JSON 參數結構
type VideoConvertArgs struct {
	InputDir     string `json:"input_dir"`
	OutputDir    string `json:"output_dir"`
	TargetFormat string `json:"target_format,omitempty"` // 預設 mp4
	Workers      int    `json:"workers,omitempty"`       // 預設 2
}

func (t *VideoConverterTool) Name() string { return "convert_videos" }

func (t *VideoConverterTool) Definition() api.Tool {
	var tool api.Tool
	jsonStr := `{
		"type": "function",
		"function": {
			"name": "convert_videos",
			"description": "Convert video files in a directory to a specified format using FFmpeg. It automatically detects codecs to optimize speed (stream copy vs transcoding).",
			"parameters": {
				"type": "object",
				"properties": {
					"input_dir": {
						"type": "string",
						"description": "The source directory path containing video files."
					},
					"output_dir": {
						"type": "string",
						"description": "The destination directory path for converted files."
					},
					"target_format": {
						"type": "string",
						"description": "Target video format extension (e.g., mp4, mkv, mov). Default is mp4.",
						"enum": ["mp4", "mkv", "mov", "avi", "webm"]
					},
					"workers": {
						"type": "integer",
						"description": "Number of concurrent conversion tasks. Default is 2."
					}
				},
				"required": ["input_dir", "output_dir"]
			}
		}
	}`
	json.Unmarshal([]byte(jsonStr), &tool)
	return tool
}

func (t *VideoConverterTool) Run(argsJSON string) (string, error) {
	// 1. 解析參數
	var args VideoConvertArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	// 設定預設值
	if args.TargetFormat == "" {
		args.TargetFormat = "mp4"
	}
	if args.Workers <= 0 {
		args.Workers = 2
	}
	// 確保格式有 "."
	if !strings.HasPrefix(args.TargetFormat, ".") {
		args.TargetFormat = "." + args.TargetFormat
	}

	// 2. 檢查與建立資料夾
	if _, err := os.Stat(args.InputDir); os.IsNotExist(err) {
		return fmt.Sprintf("Error: Input directory '%s' does not exist.", args.InputDir), nil
	}
	if err := os.MkdirAll(args.OutputDir, 0755); err != nil {
		return fmt.Sprintf("Error: Failed to create output directory: %v", err), nil
	}

	// 3. 掃描檔案
	validExtensions := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
		".flv": true, ".wmv": true, ".m4v": true, ".webm": true,
		".ts": true,
	}

	var files []string
	err := filepath.WalkDir(args.InputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if validExtensions[strings.ToLower(filepath.Ext(path))] {
				files = append(files, path)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Sprintf("Error scanning directory: %v", err), nil
	}
	if len(files) == 0 {
		return "No video files found in the input directory.", nil
	}

	// 4. 開始並發處理
	var wg sync.WaitGroup
	sem := make(chan struct{}, args.Workers) // Semaphore 控制並發

	var successCount int32 = 0
	var failCount int32 = 0
	var errors []string
	var errMutex sync.Mutex // 保護 errors slice

	for _, file := range files {
		wg.Add(1)
		sem <- struct{}{} // 獲取令牌

		go func(inputFile string) {
			defer wg.Done()
			defer func() { <-sem }() // 釋放令牌

			// 計算輸出路徑
			fileName := filepath.Base(inputFile)
			ext := filepath.Ext(fileName)
			nameWithoutExt := strings.TrimSuffix(fileName, ext)
			outputFile := filepath.Join(args.OutputDir, nameWithoutExt+args.TargetFormat)

			// 執行轉檔邏輯
			err := t.convertSingleFile(inputFile, outputFile, args.TargetFormat)
			if err != nil {
				atomic.AddInt32(&failCount, 1)
				errMutex.Lock()
				errors = append(errors, fmt.Sprintf("%s: %v", fileName, err))
				errMutex.Unlock()
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}(file)
	}

	wg.Wait()

	// 5. 回傳總結報告給 LLM
	resultMsg := fmt.Sprintf("Batch processing complete.\n- Total Files: %d\n- Success: %d\n- Failed: %d\n- Target: %s",
		len(files), successCount, failCount, args.TargetFormat)

	if len(errors) > 0 {
		resultMsg += fmt.Sprintf("\n\nErrors encountered:\n%s", strings.Join(errors, "\n"))
	}

	return resultMsg, nil
}

// convertSingleFile 封裝單一檔案的 FFmpeg 邏輯
func (t *VideoConverterTool) convertSingleFile(inputFile, outputFile, targetExt string) error {
	// 偵測編碼
	vCodec, _ := t.getCodecName(inputFile, "v")
	aCodec, _ := t.getCodecName(inputFile, "a")

	vCmd := "libx264"
	aCmd := "aac"

	// 智慧判斷策略
	if vCodec == "h264" {
		vCmd = "copy"
	}
	if aCodec == "aac" {
		aCmd = "copy"
	}

	// 若目標容器不支援現代編碼 (如 avi)，強制轉碼
	isModernContainer := map[string]bool{".mp4": true, ".mkv": true, ".mov": true, ".m4v": true}
	if !isModernContainer[strings.ToLower(targetExt)] {
		vCmd = "libx264"
		aCmd = "aac"
	}

	cmd := exec.Command("ffmpeg",
		"-y", "-v", "error",
		"-i", inputFile,
		"-c:v", vCmd,
		"-c:a", aCmd,
		outputFile,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %s", string(output))
	}
	return nil
}

// getCodecName 輔助函數 (FFprobe)
func (t *VideoConverterTool) getCodecName(inputFile string, streamType string) (string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", streamType+":0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputFile,
	)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
