## GoogleSearch
Description: 當使用者想要搜尋網路資訊、查找資料或新聞時使用。
Command: open "https://www.google.com/search?q={{query}}"

## SystemUpdate
Description: 更新系統套件或軟體時使用。
Command: sudo apt-get update && sudo apt-get upgrade -y

## PythonScript
Description: 執行特定的 Python 數據處理腳本。
Command: docker run --rm -v $(pwd):/app python:3.9 python /app/script.py "{{args}}"

## listDir
Description: 列出指定目錄下的所有檔案和資料夾。
Command: dir/w {{args}}